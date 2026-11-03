use crate::{data::DataStore, sessions::SessionManager};
use serde_json::{Value, json};
use std::{
    collections::HashMap,
    io::{self, BufRead, BufReader, Read, Write},
    net::{TcpListener, TcpStream},
    path::PathBuf,
    sync::{
        Arc, Mutex,
        atomic::{AtomicBool, Ordering},
    },
    thread,
    time::Duration,
};
use tauri::{AppHandle, Emitter, Manager};

mod stdio;
mod transport;
pub use stdio::run_stdio;
use transport::{Listener, spawn_listener};

const SUPPORTED_VERSIONS: &[&str] = &["2025-06-18", "2025-03-26", "2024-11-05"];

const MAX_HEADER_BYTES: usize = 16 * 1024;
const MAX_BODY_BYTES: usize = 4 * 1024 * 1024;
const MCP_TOOLS: &[(&str, &str, &str)] = &[
    (
        "shellink_list_profiles",
        "List all configured Shellink connection profiles (SSH or command).",
        r#"{"type":"object","properties":{}}"#,
    ),
    (
        "shellink_create_profile",
        "Create a Shellink connection profile. SSH profiles use host, port, username and authentication fields.",
        r#"{"type":"object","properties":{"name":{"type":"string","description":"Profile display name"},"connectType":{"type":"string","enum":["ssh","command"]},"host":{"type":"string"},"port":{"type":"integer","minimum":1,"maximum":65535},"username":{"type":"string"},"authType":{"type":"string","enum":["password","password-mfa","key","keyboard-interactive"]},"password":{"type":"string","description":"Plaintext password (encrypted by Shellink)"},"privateKey":{"type":"string","description":"Private key (encrypted by Shellink)"},"passphrase":{"type":"string"},"command":{"type":"string","description":"Command for connectType=command"},"term":{"type":"string"}},"required":["name","connectType"]}"#,
    ),
    (
        "shellink_list_sessions",
        "List all Shellink sessions (active and historical).",
        r#"{"type":"object","properties":{}}"#,
    ),
    (
        "shellink_create_session",
        "Open a new session for a profile and start connecting.",
        r#"{"type":"object","properties":{"profileId":{"type":"string","description":"ID of the profile to connect with"}},"required":["profileId"]}"#,
    ),
    (
        "shellink_session_state",
        "Get the current state, mode and recent output of a session.",
        r#"{"type":"object","properties":{"id":{"type":"string"}},"required":["id"]}"#,
    ),
    (
        "shellink_read_output",
        "Read the latest plain terminal logs in chronological order. limit is the number of recent text lines (default 200, max 10000), including an unfinished final line such as a shell prompt. Lines are joined across PTY chunks and are not cut by character count. Each call returns a fresh snapshot without consuming output or tracking a cursor, so readers do not affect each other. hasEarlier indicates omitted older lines. Reads return immediately; repeat to observe later output. active describes the session, not command completion. This is a log snapshot, not a rendered terminal screen.",
        r#"{"type":"object","properties":{"id":{"type":"string"},"limit":{"type":"integer","minimum":1,"maximum":10000,"default":200}},"required":["id"]}"#,
    ),
    (
        "shellink_send_input",
        "Send text to an AUTO session. appendNewline defaults to true for commands. Set it to false for raw keys: Ctrl-C=\\u0003, Tab=\\t, Escape=\\u001b, Up=\\u001b[A. Empty text with appendNewline=true sends Enter. Use read_output to observe the result.",
        r#"{"type":"object","properties":{"id":{"type":"string"},"text":{"type":"string","maxLength":262144},"appendNewline":{"type":"boolean","default":true}},"required":["id","text"]}"#,
    ),
    (
        "shellink_set_mode",
        "Switch a session between AUTO (MCP input allowed) and MANUAL (human typing only). MCP may switch back to AUTO to resume work; this is a collaboration mode, not an authorization boundary.",
        r#"{"type":"object","properties":{"id":{"type":"string"},"mode":{"type":"string","enum":["AUTO","MANUAL"]}},"required":["id","mode"]}"#,
    ),
    (
        "shellink_close_session",
        "Close/terminate a session.",
        r#"{"type":"object","properties":{"id":{"type":"string"}},"required":["id"]}"#,
    ),
];

pub struct Server {
    listener: Mutex<Option<Listener>>,
    bridge: Mutex<Option<Listener>>,
    discovery: Mutex<Option<(PathBuf, String)>>,
    configuration: Mutex<()>,
    bridge_error: Mutex<Option<String>>,
    error: Mutex<Option<String>>,
    messages: Mutex<Vec<Value>>,
}

impl Server {
    pub fn new() -> Self {
        Self {
            listener: Mutex::new(None),
            bridge: Mutex::new(None),
            discovery: Mutex::new(None),
            configuration: Mutex::new(()),
            bridge_error: Mutex::new(None),
            error: Mutex::new(None),
            messages: Mutex::new(Vec::new()),
        }
    }

    pub fn start(&self, app: AppHandle) {
        let db = app.state::<DataStore>();
        if let Err(error) = self.start_bridge(&app, &db) {
            *self.bridge_error.lock().unwrap() = Some(error);
        }
        if let Err(error) = self.reconfigure(&app, &db) {
            *self.error.lock().unwrap() = Some(error);
        }
    }

    pub fn info(&self, db: &DataStore) -> Result<Value, String> {
        let mut info = db.mcp_info()?;
        let enabled =
            info.get("enabled").and_then(Value::as_bool).unwrap_or(true) && http_enabled();
        let error = self.error.lock().unwrap().clone();
        apply_server_info(&mut info, enabled, self.is_running(), error.as_deref());
        info["stdioRunning"] = json!(
            self.bridge
                .lock()
                .unwrap()
                .as_ref()
                .is_some_and(|listener| listener.running.load(Ordering::Acquire))
        );
        if let Some(error) = self.bridge_error.lock().unwrap().as_ref() {
            info["stdioError"] = json!(error);
        }
        Ok(info)
    }

    fn start_bridge(&self, app: &AppHandle, db: &DataStore) -> Result<(), String> {
        let listener = TcpListener::bind(("127.0.0.1", 0)).map_err(|e| e.to_string())?;
        let address = listener.local_addr().map_err(|e| e.to_string())?;
        let listener = spawn_listener(listener, app.clone(), true).map_err(|e| e.to_string())?;
        match stdio::write_discovery(db.data_home(), address) {
            Ok(discovery) => {
                *self.discovery.lock().unwrap() = Some(discovery);
                *self.bridge.lock().unwrap() = Some(listener);
                Ok(())
            }
            Err(error) => {
                listener.stop();
                Err(error)
            }
        }
    }

    pub fn update_config(
        &self,
        app: &AppHandle,
        db: &DataStore,
        params: &Value,
    ) -> Result<Value, String> {
        db.mcp_config(params)?;
        self.reconfigure(app, db)
    }

    pub fn reconfigure(&self, app: &AppHandle, db: &DataStore) -> Result<Value, String> {
        let _configuration = self.configuration.lock().unwrap();
        self.stop_listener();
        let mut info = db.mcp_info()?;
        let enabled =
            info.get("enabled").and_then(Value::as_bool).unwrap_or(true) && http_enabled();
        *self.error.lock().unwrap() = None;

        if enabled {
            let host = bind_host();
            let port = http_port();
            match TcpListener::bind((host.as_str(), port)) {
                Ok(listener) => {
                    *self.listener.lock().unwrap() = Some(
                        spawn_listener(listener, app.clone(), false).map_err(|e| e.to_string())?,
                    );
                }
                Err(error) => {
                    *self.error.lock().unwrap() =
                        Some(format!("无法监听 MCP HTTP {host}:{port}: {error}"));
                }
            }
        }

        let error = self.error.lock().unwrap().clone();
        apply_server_info(&mut info, enabled, self.is_running(), error.as_deref());
        Ok(info)
    }

    pub fn stop(&self) {
        let _configuration = self.configuration.lock().unwrap();
        self.stop_listener();
        if let Some(listener) = self.bridge.lock().unwrap().take() {
            listener.stop();
        }
        if let Some((path, contents)) = self.discovery.lock().unwrap().take() {
            if std::fs::read_to_string(&path).ok().as_deref() == Some(&contents) {
                let _ = std::fs::remove_file(path);
            }
        }
    }

    pub fn messages(&self) -> Value {
        let mut messages = self.messages.lock().unwrap().clone();
        messages.reverse();
        Value::Array(messages)
    }

    pub fn clear_messages(&self) {
        self.messages.lock().unwrap().clear();
    }

    fn receive_message(&self, app: &AppHandle, data: Value) -> Value {
        let message = json!({
            "id": format!("webhook-{}", crate::data::session_now()),
            "receivedAt": crate::data::session_now(),
            "data": data,
        });
        let mut messages = self.messages.lock().unwrap();
        messages.push(message.clone());
        if messages.len() > 200 {
            let keep_from = messages.len() - 200;
            messages.drain(..keep_from);
        }
        drop(messages);
        let event = json!({"event":"webhook.received","data":message});
        if app
            .try_state::<Arc<crate::bridge::Backend>>()
            .is_none_or(|backend| backend.allows_event(&event))
        {
            let _ = app.emit("shellink:event", event);
        }
        message
    }

    fn stop_listener(&self) {
        let listener = self.listener.lock().unwrap().take();
        if let Some(listener) = listener {
            listener.stop();
        }
    }

    fn is_running(&self) -> bool {
        self.listener
            .lock()
            .unwrap()
            .as_ref()
            .is_some_and(|listener| listener.running.load(Ordering::Acquire))
    }
}

fn apply_server_info(info: &mut Value, enabled: bool, running: bool, error: Option<&str>) {
    let url = mcp_url();
    info["url"] = Value::String(url.clone());
    info["httpEnabled"] = Value::Bool(enabled);
    info["running"] = Value::Bool(running);
    info["clientConfig"]["mcpServers"]["shellink"]["url"] = Value::String(url);
    if let Some(error) = error {
        info["error"] = Value::String(error.to_owned());
    } else if let Some(object) = info.as_object_mut() {
        object.remove("error");
    }
}

fn bind_host() -> String {
    std::env::var("SHELLINK_HOST")
        .ok()
        .filter(|host| !host.trim().is_empty())
        .unwrap_or_else(|| "127.0.0.1".into())
}

fn http_port() -> u16 {
    std::env::var("SHELLINK_PORT")
        .ok()
        .and_then(|port| port.parse().ok())
        .filter(|port| *port != 0)
        .unwrap_or(7070)
}

fn http_enabled() -> bool {
    http_enabled_value(std::env::var("SHELLINK_HTTP_ENABLED").ok().as_deref())
}

fn http_enabled_value(value: Option<&str>) -> bool {
    !value.is_some_and(|value| {
        matches!(
            value.trim().to_ascii_lowercase().as_str(),
            "false" | "0" | "no" | "off"
        )
    })
}

fn mcp_url() -> String {
    let host = bind_host();
    let display_host = if host == "0.0.0.0" {
        "127.0.0.1"
    } else if host == "::" {
        "::1"
    } else {
        &host
    };
    let display_host = if display_host.contains(':') && !display_host.starts_with('[') {
        format!("[{display_host}]")
    } else {
        display_host.to_owned()
    };
    format!("http://{display_host}:{}/shellink/mcp", http_port())
}

fn process_request(
    app: &AppHandle,
    request: Value,
    stop: &AtomicBool,
    internal: bool,
) -> Result<Option<Value>, (Value, i64, String)> {
    process_request_with(request, internal, |method, params| match method {
        "tools/call" => {
            let db = app.state::<DataStore>();
            let manager = app.state::<Arc<SessionManager>>();
            call_tool_with(&db, &manager, params, stop, |profile_id| {
                manager.create(app, &db, profile_id, "", "mcp", "AUTO", 0, 0)
            })
        }
        _ => Err((-32601, format!("method not found: {method}"))),
    })
}

fn process_request_with(
    request: Value,
    legacy_stdio: bool,
    dispatch: impl FnOnce(&str, &Value) -> Result<Value, (i64, String)>,
) -> Result<Option<Value>, (Value, i64, String)> {
    let Some(object) = request.as_object() else {
        return Err((Value::Null, -32600, "invalid request".into()));
    };
    let id = object.get("id").cloned().unwrap_or(Value::Null);
    if object.get("jsonrpc").and_then(Value::as_str) != Some("2.0") {
        return Err((id, -32600, "invalid request".into()));
    }
    let Some(method) = object.get("method").and_then(Value::as_str) else {
        return Err((id, -32600, "method must be a string".into()));
    };
    if !(id.is_string() || id.is_number() || id.is_null()) {
        return Err((Value::Null, -32600, "invalid request id".into()));
    }
    if !object.contains_key("id") {
        return Ok(None);
    }
    let params = object.get("params").unwrap_or(&Value::Null);
    if !params.is_null() && !params.is_object() {
        return Err((id, -32602, "params must be an object".into()));
    }
    let result = match method {
        "initialize" => match params.get("protocolVersion").and_then(Value::as_str) {
            Some(requested) => {
                let version = if SUPPORTED_VERSIONS.contains(&requested)
                    && (legacy_stdio || requested != "2024-11-05")
                {
                    requested
                } else {
                    SUPPORTED_VERSIONS[0]
                };
                Ok(
                    json!({"protocolVersion":version,"serverInfo":{"name":"shellink","version":env!("CARGO_PKG_VERSION")},"capabilities":{"tools":{}},"instructions":"Use profiles to open or reuse terminal sessions. AUTO/MANUAL are switchable collaboration modes. read_output returns the latest 200 text lines on every call; only id is required, and no cursor is needed. Increase limit for more lines. Repeat reads to observe later output; active describes the session, not command completion. Set appendNewline=false for raw terminal keys. SSH MFA must be completed in the Desktop dialog."}),
                )
            }
            None => Err((-32602, "initialize requires protocolVersion".into())),
        },
        "ping" => Ok(json!({})),
        "tools/list" => Ok(json!({"tools": tools()})),
        "tools/call" => dispatch(method, params),
        _ => Err((-32601, format!("method not found: {method}"))),
    };
    match result {
        Ok(result) => Ok(Some(json!({"jsonrpc":"2.0","id":id,"result":result}))),
        Err((code, message)) => Err((id, code, message)),
    }
}

fn input_schema(schema: &str) -> Value {
    let mut schema: Value = serde_json::from_str(schema).expect("valid built-in MCP schema");
    schema["additionalProperties"] = json!(false);
    for (name, property) in schema["properties"].as_object_mut().unwrap() {
        if property["type"] == "string" && name != "text" {
            property["minLength"] = json!(1);
        }
    }
    schema
}

fn tools() -> Vec<Value> {
    MCP_TOOLS.iter().map(|(name, description, schema)| {
        let read_only = matches!(*name, "shellink_list_profiles" | "shellink_list_sessions" | "shellink_session_state" | "shellink_read_output");
        json!({"name":name,"description":description,"inputSchema":input_schema(schema),"outputSchema":output_schema(name),"annotations":{"readOnlyHint":read_only,"destructiveHint":!read_only,"idempotentHint":read_only || matches!(*name,"shellink_set_mode"|"shellink_close_session"),"openWorldHint":true}})
    }).collect()
}

fn output_schema(name: &str) -> Value {
    let properties = match name {
        "shellink_list_profiles" => json!({"profiles":{"type":"array","items":{"type":"object"}}}),
        "shellink_list_sessions" => json!({"sessions":{"type":"array","items":{"type":"object"}}}),
        "shellink_create_profile" => json!({"id":{"type":"string"},"name":{"type":"string"}}),
        "shellink_create_session" => {
            json!({"id":{"type":"string"},"state":{"type":"string"},"mode":{"type":"string"},"target":{"type":"string"}})
        }
        "shellink_session_state" => {
            json!({"id":{"type":"string"},"state":{"type":"string"},"mode":{"type":"string"},"active":{"type":"boolean"},"lastSeq":{"type":"integer"},"recentOutput":{"type":"string"},"exitCode":{"type":["integer","null"]}})
        }
        "shellink_read_output" => {
            json!({"text":{"type":"string"},"hasEarlier":{"type":"boolean"},"active":{"type":"boolean"}})
        }
        "shellink_send_input" => json!({"ok":{"type":"boolean"}}),
        "shellink_set_mode" => json!({"ok":{"type":"boolean"},"mode":{"type":"string"}}),
        "shellink_close_session" => json!({"closed":{"type":"boolean"}}),
        _ => unreachable!(),
    };
    let required: Vec<_> = properties.as_object().unwrap().keys().cloned().collect();
    let mut properties = properties;
    properties["error"] = json!({"type":"object","properties":{"code":{"type":"string"},"message":{"type":"string"}},"required":["code","message"]});
    json!({"type":"object","properties":properties,"anyOf":[{"required":required},{"required":["error"]}]})
}

fn validate_arguments(args: &Value, schema: &Value) -> Result<(), String> {
    let object = args.as_object().ok_or("tool arguments must be an object")?;
    if let Some(required) = schema["required"].as_array() {
        for field in required.iter().filter_map(Value::as_str) {
            if !object.contains_key(field) {
                return Err(format!("{field} is required"));
            }
        }
    }
    let properties = schema["properties"].as_object().unwrap();
    for (field, value) in object {
        let property = properties
            .get(field)
            .ok_or_else(|| format!("unknown argument: {field}"))?;
        let valid_type = match property["type"].as_str() {
            Some("string") => value.is_string(),
            Some("integer") => value.as_i64().is_some(),
            Some("boolean") => value.is_boolean(),
            _ => true,
        };
        if !valid_type {
            return Err(format!(
                "{field} must be a {}",
                property["type"].as_str().unwrap_or("valid value")
            ));
        }
        if let Some(variants) = property["enum"].as_array() {
            if !variants.contains(value) {
                return Err(format!("{field} must be one of {}", property["enum"]));
            }
        }
        if let Some(number) = value.as_i64() {
            if property["minimum"].as_i64().is_some_and(|min| number < min)
                || property["maximum"].as_i64().is_some_and(|max| number > max)
            {
                return Err(format!("{field} is outside its allowed range"));
            }
        }
        if let Some(text) = value.as_str() {
            let length = text.chars().count() as u64;
            if property["minLength"]
                .as_u64()
                .is_some_and(|min| length < min)
                || property["maxLength"]
                    .as_u64()
                    .is_some_and(|max| length > max)
            {
                return Err(format!("{field} has an invalid length"));
            }
        }
    }
    Ok(())
}

fn call_tool_with(
    db: &DataStore,
    manager: &SessionManager,
    params: &Value,
    stop: &AtomicBool,
    create: impl FnOnce(&str) -> Result<Value, String>,
) -> Result<Value, (i64, String)> {
    let name = params
        .get("name")
        .and_then(Value::as_str)
        .ok_or_else(|| (-32602, "tools/call requires a tool name".into()))?;
    let (_, _, schema) = MCP_TOOLS
        .iter()
        .find(|(tool, _, _)| *tool == name)
        .ok_or_else(|| (-32602, format!("unknown tool: {name}")))?;
    let args = params
        .get("arguments")
        .cloned()
        .unwrap_or_else(|| json!({}));
    if let Err(error) = validate_arguments(&args, &input_schema(schema)) {
        return Ok(tool_error("INVALID_ARGUMENT", &error));
    }
    if stop.load(Ordering::Acquire) {
        return Ok(tool_error("CANCELLED", "MCP listener is stopping"));
    }
    let id = args["id"].as_str().unwrap_or_default();
    let result = match name {
        "shellink_list_profiles" => db
            .profiles(None)
            .map(|profiles| json!({"profiles":profiles})),
        "shellink_create_profile" => {
            let required = if args["connectType"] == "command" {
                &["command"][..]
            } else {
                &["host", "username"][..]
            };
            for field in required {
                if args
                    .get(*field)
                    .and_then(Value::as_str)
                    .is_none_or(|v| v.trim().is_empty())
                {
                    return Ok(tool_error(
                        "INVALID_ARGUMENT",
                        &format!("{field} is required for this connectType"),
                    ));
                }
            }
            db.profile_create(&args)
        }
        "shellink_list_sessions" => manager
            .list(db)
            .map(|sessions| json!({"sessions":sessions})),
        "shellink_create_session" => create(args["profileId"].as_str().unwrap()),
        "shellink_session_state" => manager.state(db, id),
        "shellink_read_output" => read_output(db, manager, &args),
        "shellink_send_input" => manager.input(
            db,
            id,
            args["text"].as_str().unwrap(),
            args["appendNewline"].as_bool().unwrap_or(true),
            false,
        ),
        "shellink_set_mode" => manager.set_mode(db, id, args["mode"].as_str().unwrap()),
        "shellink_close_session" => manager.close(db, id),
        _ => unreachable!(),
    };
    Ok(match result {
        Ok(value) => tool_success(value),
        Err(error) => {
            let code = match error.as_str() {
                "Session not found" => "SESSION_NOT_FOUND",
                "Profile not found" => "PROFILE_NOT_FOUND",
                "Session is in MANUAL mode" => "MANUAL_MODE",
                "请先在 SSH 认证窗口中完成 MFA/2FA 验证" => "AUTH_REQUIRED",
                "request cancelled" => "CANCELLED",
                _ => "OPERATION_FAILED",
            };
            tool_error(code, &error)
        }
    })
}

fn read_output(db: &DataStore, manager: &SessionManager, args: &Value) -> Result<Value, String> {
    let id = args["id"].as_str().unwrap();
    let state = manager.state(db, id)?;
    let mut result = db.read_output(id, args["limit"].as_u64().unwrap_or(200) as usize)?;
    result["active"] = state["active"].clone();
    Ok(result)
}

fn tool_success(value: Value) -> Value {
    json!({"content":[{"type":"text","text":value.to_string()}],"structuredContent":value,"isError":false})
}

fn tool_error(code: &str, message: &str) -> Value {
    let error = json!({"error":{"code":code,"message":message}});
    json!({"isError":true,"content":[{"type":"text","text":error.to_string()}],"structuredContent":error})
}

#[cfg(test)]
mod tests {
    use super::*;
    use transport::{parse_content_length, read_request};

    fn invoke(db: &DataStore, manager: &SessionManager, name: &str, args: Value) -> Value {
        call_tool_with(
            db,
            manager,
            &json!({"name":name,"arguments":args}),
            &AtomicBool::new(false),
            |_| unreachable!(),
        )
        .unwrap()
    }

    #[test]
    fn tools_expose_typed_results_and_no_screen_emulator() {
        let schemas = tools();
        assert_eq!(schemas.len(), 9);
        assert!(!schemas.iter().any(|tool| tool["name"] == "shellink_screen"));
        let read = schemas
            .iter()
            .find(|tool| tool["name"] == "shellink_read_output")
            .unwrap();
        assert_eq!(read["annotations"]["readOnlyHint"], true);
        assert_eq!(read["inputSchema"]["properties"]["limit"]["default"], 200);
        assert_eq!(
            read["inputSchema"]["properties"].as_object().unwrap().len(),
            2
        );
        assert_eq!(read["inputSchema"]["required"], json!(["id"]));
        assert_eq!(read["outputSchema"]["type"], "object");
        let input = schemas
            .iter()
            .find(|tool| tool["name"] == "shellink_send_input")
            .unwrap();
        assert_eq!(input["inputSchema"]["required"], json!(["id", "text"]));
        assert_eq!(
            input["inputSchema"]["properties"]["appendNewline"]["default"],
            true
        );
    }

    #[test]
    fn structured_errors_and_ai_mode_switching() {
        let db = DataStore::test_store(PathBuf::new());
        let manager = SessionManager::new();
        db.insert_session("s", "p", "Profile", "local", 120, 36, "", "test", "MANUAL")
            .unwrap();
        let manual = invoke(
            &db,
            &manager,
            "shellink_send_input",
            json!({"id":"s","text":"pwd"}),
        );
        assert_eq!(manual["structuredContent"]["error"]["code"], "MANUAL_MODE");
        let switched = invoke(
            &db,
            &manager,
            "shellink_set_mode",
            json!({"id":"s","mode":"AUTO"}),
        );
        assert_eq!(switched["structuredContent"]["mode"], "AUTO");
        assert_eq!(db.session_mode("s").unwrap(), "AUTO");
        for args in [
            json!({"id":"s","limit":"200"}),
            json!({"id":"s","maxChars":16000}),
        ] {
            let invalid = invoke(&db, &manager, "shellink_read_output", args);
            assert_eq!(
                invalid["structuredContent"]["error"]["code"],
                "INVALID_ARGUMENT"
            );
        }
        let missing = invoke(
            &db,
            &manager,
            "shellink_session_state",
            json!({"id":"missing"}),
        );
        assert_eq!(
            missing["structuredContent"]["error"]["code"],
            "SESSION_NOT_FOUND"
        );
    }

    #[test]
    fn output_reads_return_independent_recent_snapshots() {
        let db = DataStore::test_store(PathBuf::new());
        let manager = SessionManager::new();
        db.insert_session("s", "p", "Profile", "local", 120, 36, "", "test", "AUTO")
            .unwrap();
        let empty = invoke(&db, &manager, "shellink_read_output", json!({"id":"s"}));
        assert_eq!(
            empty["structuredContent"],
            json!({"text":"","hasEarlier":false,"active":true})
        );
        db.add_output("s", "first\n").unwrap();
        db.add_output("s", "latest\n").unwrap();
        for _ in 0..2 {
            let result = invoke(
                &db,
                &manager,
                "shellink_read_output",
                json!({"id":"s","limit":1}),
            );
            assert_eq!(
                result["structuredContent"],
                json!({"text":"latest\n","hasEarlier":true,"active":true})
            );
        }
        db.update_session_state("s", "DISCONNECTED", Some("test"))
            .unwrap();
        let closed = invoke(&db, &manager, "shellink_read_output", json!({"id":"s"}));
        assert_eq!(
            closed["structuredContent"],
            json!({"text":"first\nlatest\n","hasEarlier":false,"active":false})
        );
        let cancelled = call_tool_with(
            &db,
            &manager,
            &json!({"name":"shellink_read_output","arguments":{"id":"s"}}),
            &AtomicBool::new(true),
            |_| unreachable!(),
        )
        .unwrap();
        assert_eq!(cancelled["structuredContent"]["error"]["code"], "CANCELLED");
    }

    #[test]
    fn reads_request_body_when_it_arrives_in_multiple_writes() {
        let listener = TcpListener::bind("127.0.0.1:0").unwrap();
        let address = listener.local_addr().unwrap();
        let body = br#"{"jsonrpc":"2.0","id":1,"method":"ping"}"#;
        let client = thread::spawn(move || {
            let mut stream = TcpStream::connect(address).unwrap();
            write!(
                stream,
                "POST /shellink/mcp HTTP/1.1\r\nContent-Length: {}\r\n\r\n",
                body.len()
            )
            .unwrap();
            stream.flush().unwrap();
            thread::sleep(Duration::from_millis(10));
            stream.write_all(body).unwrap();
        });
        let (mut stream, _) = listener.accept().unwrap();
        let request = read_request(&mut stream).unwrap();
        client.join().unwrap();
        assert_eq!(request.body, body);
    }

    #[test]
    fn rejects_oversized_and_missing_length_bodies() {
        assert_eq!(
            parse_content_length(None),
            Err((411, "Content-Length is required".into()))
        );
        assert_eq!(
            parse_content_length(Some("4194305")),
            Err((413, "request body too large".into()))
        );
    }

    #[test]
    fn negotiates_versions_and_handles_json_rpc_errors() {
        for (requested, internal, expected) in [
            ("2025-03-26", false, "2025-03-26"),
            ("2024-11-05", true, "2024-11-05"),
            ("2024-11-05", false, "2025-06-18"),
            ("2099-01-01", false, "2025-06-18"),
        ] {
            let response = process_request_with(json!({"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":requested}}), internal, |_,_| unreachable!()).unwrap().unwrap();
            assert_eq!(response["result"]["protocolVersion"], expected);
        }
        let ping = process_request_with(
            json!({"jsonrpc":"2.0","id":"p1","method":"ping"}),
            false,
            |_, _| unreachable!(),
        )
        .unwrap()
        .unwrap();
        assert_eq!(ping["result"], json!({}));
        let error = process_request_with(
            json!({"jsonrpc":"2.0","id":7,"method":"no/such/method"}),
            false,
            |_, _| unreachable!(),
        )
        .unwrap_err();
        assert_eq!(error.1, -32601);
        assert!(
            process_request_with(
                json!({"jsonrpc":"2.0","method":"notifications/initialized"}),
                false,
                |_, _| unreachable!()
            )
            .unwrap()
            .is_none()
        );
    }

    #[test]
    fn respects_http_environment_switch_case_insensitively() {
        assert!(!http_enabled_value(Some("FALSE")));
        assert!(!http_enabled_value(Some(" off ")));
        assert!(http_enabled_value(Some("true")));
        assert!(http_enabled_value(None));
    }
}
