use crate::{data::DataStore, sessions::SessionManager};
use serde_json::{Value, json};
use std::{
    collections::HashMap,
    io::{self, BufRead, BufReader, Read, Write},
    net::{TcpListener, TcpStream},
    sync::{
        Arc, Mutex,
        atomic::{AtomicBool, Ordering},
    },
    thread,
    time::Duration,
};
use tauri::{AppHandle, Emitter, Manager};

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
        r#"{"type":"object","properties":{"name":{"type":"string","description":"Profile display name"},"connectType":{"type":"string","enum":["ssh","command"]},"host":{"type":"string"},"port":{"type":"integer"},"username":{"type":"string"},"authType":{"type":"string","enum":["password","key"]},"password":{"type":"string","description":"Plaintext password (encrypted by Shellink)"},"privateKey":{"type":"string","description":"Private key (encrypted by Shellink)"},"passphrase":{"type":"string"},"command":{"type":"string","description":"Command for connectType=command"},"term":{"type":"string"}},"required":["name","connectType"]}"#,
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
        "Read the terminal output (plain text) of a session.",
        r#"{"type":"object","properties":{"id":{"type":"string"},"limit":{"type":"integer","description":"Max number of history chunks"}},"required":["id"]}"#,
    ),
    (
        "shellink_send_input",
        "Send text input to a session (as if typed into the terminal).",
        r#"{"type":"object","properties":{"id":{"type":"string"},"text":{"type":"string"}},"required":["id","text"]}"#,
    ),
    (
        "shellink_close_session",
        "Close/terminate a session.",
        r#"{"type":"object","properties":{"id":{"type":"string"}},"required":["id"]}"#,
    ),
];

pub fn run_stdio() {
    let db = match DataStore::open() {
        Ok(db) => db,
        Err(error) => {
            let _ = writeln!(io::stderr(), "Shellink MCP stdio unavailable: {error}");
            return;
        }
    };
    let token = match db.mcp_info() {
        Ok(info) => info
            .get("token")
            .and_then(Value::as_str)
            .unwrap_or_default()
            .to_owned(),
        Err(error) => {
            let _ = writeln!(io::stderr(), "Shellink MCP configuration unavailable: {error}");
            return;
        }
    };
    let stdin = io::stdin();
    let mut stdout = io::BufWriter::new(io::stdout().lock());
    for line in stdin.lock().lines() {
        let line = match line {
            Ok(line) if !line.trim().is_empty() => line,
            Ok(_) => continue,
            Err(error) => {
                let _ = writeln!(io::stderr(), "Shellink MCP stdio read failed: {error}");
                break;
            }
        };
        let request: Value = match serde_json::from_str(&line) {
            Ok(value) => value,
            Err(error) => {
                let response = json!({"jsonrpc":"2.0","id":Value::Null,"error":{"code":-32700,"message":format!("parse error: {error}")}});
                let _ = writeln!(stdout, "{response}");
                let _ = stdout.flush();
                continue;
            }
        };
        let notification = !request.as_object().is_some_and(|object| object.contains_key("id"));
        match stdio_http_request(&token, &request) {
            Ok(Some(response)) if !notification => {
                let _ = writeln!(stdout, "{response}");
                let _ = stdout.flush();
            }
            Ok(_) => {}
            Err(error) if !notification => {
                let id = request.get("id").cloned().unwrap_or(Value::Null);
                let response = json!({"jsonrpc":"2.0","id":id,"error":{"code":-32000,"message":error}});
                let _ = writeln!(stdout, "{response}");
                let _ = stdout.flush();
            }
            Err(error) => {
                let _ = writeln!(io::stderr(), "Shellink MCP request failed: {error}");
            }
        }
    }
}

fn stdio_http_request(token: &str, request: &Value) -> Result<Option<Value>, String> {
    let host = bind_host();
    let connect_host = if host == "0.0.0.0" || host == "::" { "127.0.0.1" } else { host.as_str() };
    let mut stream = TcpStream::connect((connect_host, http_port()))
        .map_err(|error| format!("Desktop MCP HTTP is unavailable: {error}"))?;
    let body = serde_json::to_vec(request).map_err(|error| error.to_string())?;
    write!(
        stream,
        "POST /shellink/mcp HTTP/1.1\r\nHost: {connect_host}\r\nAuthorization: Bearer {token}\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n",
        body.len()
    )
    .map_err(|error| error.to_string())?;
    stream.write_all(&body).map_err(|error| error.to_string())?;
    stream.flush().map_err(|error| error.to_string())?;
    let mut response = Vec::new();
    stream
        .read_to_end(&mut response)
        .map_err(|error| error.to_string())?;
    let header_end = response
        .windows(4)
        .position(|window| window == b"\r\n\r\n")
        .ok_or("invalid Desktop MCP HTTP response")?;
    let header = String::from_utf8_lossy(&response[..header_end]);
    let status = header
        .lines()
        .next()
        .and_then(|line| line.split_whitespace().nth(1))
        .and_then(|value| value.parse::<u16>().ok())
        .unwrap_or(500);
    let payload = &response[header_end + 4..];
    if status == 202 || payload.is_empty() {
        return Ok(None);
    }
    let value: Value = serde_json::from_slice(payload).map_err(|error| error.to_string())?;
    if status >= 400 {
        return Err(value
            .get("error")
            .and_then(Value::as_str)
            .unwrap_or("Desktop MCP request failed")
            .to_owned());
    }
    Ok(Some(value))
}

struct Listener {
    stop: Arc<AtomicBool>,
    running: Arc<AtomicBool>,
    join: thread::JoinHandle<()>,
}

pub struct Server {
    listener: Mutex<Option<Listener>>,
    error: Mutex<Option<String>>,
    messages: Mutex<Vec<Value>>,
}

impl Server {
    pub fn new() -> Self {
        Self {
            listener: Mutex::new(None),
            error: Mutex::new(None),
            messages: Mutex::new(Vec::new()),
        }
    }

    pub fn start(&self, app: AppHandle) {
        let db = app.state::<DataStore>();
        if let Err(error) = self.reconfigure(&app, &db) {
            *self.error.lock().unwrap() = Some(error);
        }
    }

    pub fn info(&self, db: &DataStore) -> Result<Value, String> {
        let mut info = db.mcp_info()?;
        let enabled = info.get("enabled").and_then(Value::as_bool).unwrap_or(true) && http_enabled();
        let error = self.error.lock().unwrap().clone();
        apply_server_info(&mut info, enabled, self.is_running(), error.as_deref());
        Ok(info)
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
        self.stop_listener();
        let mut info = db.mcp_info()?;
        let enabled = info.get("enabled").and_then(Value::as_bool).unwrap_or(true) && http_enabled();
        *self.error.lock().unwrap() = None;

        if enabled && http_enabled() {
            let host = bind_host();
            let port = http_port();
            match TcpListener::bind((host.as_str(), port)) {
                Ok(listener) => {
                    listener.set_nonblocking(true).map_err(|e| e.to_string())?;
                    let stop = Arc::new(AtomicBool::new(false));
                    let running = Arc::new(AtomicBool::new(true));
                    let thread_stop = stop.clone();
                    let thread_running = running.clone();
                    let app = app.clone();
                    let join = thread::spawn(move || {
                        while !thread_stop.load(Ordering::Acquire) {
                            match listener.accept() {
                                Ok((stream, _)) => {
                                    let app = app.clone();
                                    thread::spawn(move || handle(stream, app));
                                }
                                Err(error) if error.kind() == std::io::ErrorKind::WouldBlock => {
                                    thread::sleep(Duration::from_millis(25));
                                }
                                Err(_) => break,
                            }
                        }
                        thread_running.store(false, Ordering::Release);
                    });
                    *self.listener.lock().unwrap() = Some(Listener {
                        stop,
                        running,
                        join,
                    });
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
        self.stop_listener();
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
            listener.stop.store(true, Ordering::Release);
            let _ = listener.join.join();
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
    let display_host = if host == "0.0.0.0" || host == "::" {
        "127.0.0.1"
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

fn handle(mut stream: TcpStream, app: AppHandle) {
    let _ = stream.set_read_timeout(Some(Duration::from_secs(10)));
    let _ = stream.set_write_timeout(Some(Duration::from_secs(10)));
    let request = match read_request(&mut stream) {
        Ok(request) => request,
        Err((status, message)) => {
            respond_error(&mut stream, status, &message);
            return;
        }
    };
    if request.method != "POST"
        || !matches!(
            request.path.as_str(),
            "/shellink/mcp" | "/shellink/webhook/callback"
        )
    {
        respond_error(&mut stream, 404, "not found");
        return;
    }

    if request.path == "/shellink/webhook/callback" {
        let body = match serde_json::from_slice::<Value>(&request.body) {
            Ok(value) => value,
            Err(_) => {
                respond_error(&mut stream, 400, "request body must be valid JSON");
                return;
            }
        };
        let message = app.state::<Server>().receive_message(&app, body);
        respond_json(&mut stream, 202, message);
        return;
    }
    let db = app.state::<DataStore>();
    let expected = match db.mcp_info() {
        Ok(info) => info
            .get("token")
            .and_then(Value::as_str)
            .unwrap_or("")
            .to_owned(),
        Err(error) => {
            respond_error(&mut stream, 503, &error);
            return;
        }
    };
    let auth = request
        .headers
        .get("authorization")
        .map(String::as_str)
        .unwrap_or("");
    if expected.is_empty() || auth != format!("Bearer {expected}") {
        respond_error(&mut stream, 401, "unauthorized");
        return;
    }

    let parsed = match serde_json::from_slice::<Value>(&request.body) {
        Ok(value) => value,
        Err(_) => {
            respond_json_rpc_error(&mut stream, 400, Value::Null, -32700, "parse error");
            return;
        }
    };
    let response = match process_request(&app, parsed) {
        Ok(Some(response)) => response,
        Ok(None) => {
            let _ = write!(
                stream,
                "HTTP/1.1 202 Accepted\r\nContent-Length: 0\r\nConnection: close\r\n\r\n"
            );
            return;
        }
        Err((id, code, message)) => {
            json!({"jsonrpc":"2.0","id":id,"error":{"code":code,"message":message}})
        }
    };
    respond_json(&mut stream, 200, response);
}

struct HttpRequest {
    method: String,
    path: String,
    headers: HashMap<String, String>,
    body: Vec<u8>,
}

fn read_request(stream: &mut TcpStream) -> Result<HttpRequest, (u16, String)> {
    let mut reader = BufReader::new(stream);
    let mut first = String::new();
    reader
        .read_line(&mut first)
        .map_err(|error| (400, error.to_string()))?;
    let mut parts = first.split_whitespace();
    let method = parts.next().unwrap_or_default().to_owned();
    let path = parts.next().unwrap_or_default().to_owned();
    if method.is_empty() || path.is_empty() {
        return Err((400, "invalid HTTP request line".into()));
    }

    let mut headers = HashMap::new();
    let mut header_bytes = first.len();
    loop {
        let mut line = String::new();
        let count = reader
            .read_line(&mut line)
            .map_err(|error| (400, error.to_string()))?;
        if count == 0 {
            return Err((400, "incomplete HTTP headers".into()));
        }
        header_bytes += count;
        if header_bytes > MAX_HEADER_BYTES {
            return Err((431, "HTTP headers too large".into()));
        }
        if line == "\r\n" || line == "\n" {
            break;
        }
        let Some((name, value)) = line.split_once(':') else {
            return Err((400, "invalid HTTP header".into()));
        };
        headers.insert(name.trim().to_ascii_lowercase(), value.trim().to_owned());
    }
    if headers
        .get("transfer-encoding")
        .is_some_and(|value| !value.eq_ignore_ascii_case("identity"))
    {
        return Err((400, "chunked request bodies are not supported".into()));
    }
    let content_length = parse_content_length(headers.get("content-length").map(String::as_str))?;
    let mut body = vec![0; content_length];
    reader
        .read_exact(&mut body)
        .map_err(|error| (400, format!("incomplete request body: {error}")))?;
    Ok(HttpRequest {
        method,
        path,
        headers,
        body,
    })
}

fn parse_content_length(value: Option<&str>) -> Result<usize, (u16, String)> {
    let value = value.ok_or_else(|| (411, "Content-Length is required".into()))?;
    let length = value
        .parse::<usize>()
        .map_err(|_| (400, "invalid Content-Length".into()))?;
    if length > MAX_BODY_BYTES {
        return Err((413, "request body too large".into()));
    }
    Ok(length)
}

fn process_request(app: &AppHandle, request: Value) -> Result<Option<Value>, (Value, i64, String)> {
    process_request_with(request, |method, params| match method {
        "tools/call" => call_tool(app, params),
        _ => Err((-32601, format!("method not found: {method}"))),
    })
}

fn process_request_with(
    request: Value,
    dispatch: impl FnOnce(&str, &Value) -> Result<Value, (i64, String)>,
) -> Result<Option<Value>, (Value, i64, String)> {
    let Some(object) = request.as_object() else {
        return Err((Value::Null, -32600, "invalid request".into()));
    };
    if object.get("jsonrpc").and_then(Value::as_str) != Some("2.0") {
        return Err((
            object.get("id").cloned().unwrap_or(Value::Null),
            -32600,
            "invalid request".into(),
        ));
    }
    let Some(method) = object.get("method").and_then(Value::as_str) else {
        return Err((
            object.get("id").cloned().unwrap_or(Value::Null),
            -32600,
            "method must be a string".into(),
        ));
    };
    let notification = !object.contains_key("id");
    let id = object.get("id").cloned().unwrap_or(Value::Null);
    if !notification && !(id.is_string() || id.is_number() || id.is_null()) {
        return Err((Value::Null, -32600, "invalid request id".into()));
    }
    if notification {
        if method == "notifications/initialized" || method.starts_with("notifications/") {
            return Ok(None);
        }
        return Ok(None);
    }

    let params = object.get("params").unwrap_or(&Value::Null);
    let result = match method {
        "initialize" => Ok(
            json!({"protocolVersion":"2024-11-05","serverInfo":{"name":"shellink","version":"0.2.4"},"capabilities":{"tools":{}}}),
        ),
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

fn tools() -> Vec<Value> {
    MCP_TOOLS
        .iter()
        .map(|(name, description, schema)| {
            json!({"name":name,"description":description,"inputSchema":serde_json::from_str::<Value>(schema).expect("valid built-in MCP schema")})
        })
        .collect()
}

fn call_tool(app: &AppHandle, params: &Value) -> Result<Value, (i64, String)> {
    let name = params
        .get("name")
        .and_then(Value::as_str)
        .ok_or_else(|| (-32602, "tools/call requires a tool name".into()))?;
    let arguments = params
        .get("arguments")
        .cloned()
        .unwrap_or_else(|| json!({}));
    if !arguments.is_object() {
        return Err((-32602, "tool arguments must be an object".into()));
    }
    let db = app.state::<DataStore>();
    let args = &arguments;
    let result: Result<Value, String> =
        match name {
            "shellink_list_profiles" => db.profiles(None),
            "shellink_create_profile" => db.profile_create(args),
            "shellink_list_sessions" => db.sessions(),
            "shellink_create_session" => {
                let profile_id = required_string(args, "profileId").map_err(str::to_owned);
                profile_id.and_then(|profile_id| {
                    app.try_state::<Arc<SessionManager>>()
                        .ok_or_else(|| "session manager unavailable".to_string())?
                        .create(app, &db, profile_id, "", "mcp", "AUTO", 0, 0)
                })
            }
            "shellink_session_state" => required_string(args, "id")
                .map_err(str::to_owned)
                .and_then(|id| db.session_state(id)),
            "shellink_read_output" => {
                let id = required_string(args, "id").map_err(str::to_owned);
                id.and_then(|id| {
                    let limit = args
                        .get("limit")
                        .and_then(Value::as_i64)
                        .unwrap_or(2000)
                        .clamp(1, 10_000);
                    db.history_chunks(id, 0, limit).map(|history| {
                        let text = history
                            .get("items")
                            .and_then(Value::as_array)
                            .into_iter()
                            .flatten()
                            .filter(|item| {
                                item.get("direction").and_then(Value::as_str) == Some("output")
                            })
                            .filter_map(|item| item.get("plain").and_then(Value::as_str))
                            .collect::<String>();
                        json!({"text":text})
                    })
                })
            }
            "shellink_send_input" => {
                required_string(args, "id")
                    .map_err(str::to_owned)
                    .and_then(|id| {
                        let text = required_string(args, "text").map_err(str::to_owned)?;
                        app.try_state::<Arc<SessionManager>>()
                            .ok_or_else(|| "session manager unavailable".to_string())?
                            .input(&db, id, text, true, false)
                    })
            }
            "shellink_close_session" => required_string(args, "id")
                .map_err(str::to_owned)
                .and_then(|id| {
                    app.try_state::<Arc<SessionManager>>()
                        .ok_or_else(|| "session manager unavailable".to_string())?
                        .close(&db, id)
                }),
            _ => return Ok(tool_error(&format!("unknown tool: {name}"))),
        };
    match result {
        Ok(value) => {
            let text = if let Some(text) = value.get("text").and_then(Value::as_str) {
                text.to_owned()
            } else {
                value.to_string()
            };
            Ok(json!({"content":[{"type":"text","text":text}]}))
        }
        Err(error) => Ok(tool_error(&error)),
    }
}

fn required_string<'a>(params: &'a Value, field: &str) -> Result<&'a str, &'static str> {
    params
        .get(field)
        .and_then(Value::as_str)
        .filter(|value| !value.is_empty())
        .ok_or("required string argument is missing")
}

fn tool_error(message: &str) -> Value {
    json!({"isError":true,"content":[{"type":"text","text":message}]})
}

fn respond_error(stream: &mut TcpStream, status: u16, message: &str) {
    respond_json(stream, status, json!({"error":message}));
}

fn respond_json_rpc_error(
    stream: &mut TcpStream,
    status: u16,
    id: Value,
    code: i64,
    message: &str,
) {
    respond_json(
        stream,
        status,
        json!({"jsonrpc":"2.0","id":id,"error":{"code":code,"message":message}}),
    );
}

fn respond_json(stream: &mut TcpStream, status: u16, value: Value) {
    let body = value.to_string();
    let reason = match status {
        200 => "OK",
        202 => "Accepted",
        400 => "Bad Request",
        401 => "Unauthorized",
        404 => "Not Found",
        411 => "Length Required",
        413 => "Payload Too Large",
        431 => "Request Header Fields Too Large",
        503 => "Service Unavailable",
        _ => "Error",
    };
    let _ = write!(
        stream,
        "HTTP/1.1 {status} {reason}\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{body}",
        body.len()
    );
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::net::TcpListener;

    #[test]
    fn schemas_include_required_tool_arguments() {
        let schemas = tools();
        assert_eq!(schemas.len(), 8);
        let create = schemas
            .iter()
            .find(|tool| tool["name"] == "shellink_create_session")
            .unwrap();
        assert_eq!(create["inputSchema"]["required"][0], "profileId");
        let input = schemas
            .iter()
            .find(|tool| tool["name"] == "shellink_send_input")
            .unwrap();
        assert_eq!(input["inputSchema"]["required"], json!(["id", "text"]));
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
        assert_eq!(request.path, "/shellink/mcp");
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
    fn handles_ping_and_json_rpc_errors() {
        let ping = process_request_with(
            json!({"jsonrpc":"2.0","id":"p1","method":"ping"}),
            |_, _| unreachable!(),
        )
        .unwrap()
        .unwrap();
        assert_eq!(ping["result"], json!({}));

        let error = process_request_with(
            json!({"jsonrpc":"2.0","id":7,"method":"no/such/method"}),
            |_, _| unreachable!(),
        )
        .unwrap_err();
        assert_eq!(
            error,
            (json!(7), -32601, "method not found: no/such/method".into())
        );
    }

    #[test]
    fn accepts_initialized_notification_without_a_response() {
        let response = process_request_with(
            json!({"jsonrpc":"2.0","method":"notifications/initialized"}),
            |_, _| unreachable!(),
        )
        .unwrap();
        assert!(response.is_none());
    }

    #[test]
    fn respects_http_environment_switch_case_insensitively() {
        assert!(!http_enabled_value(Some("FALSE")));
        assert!(!http_enabled_value(Some(" off ")));
        assert!(http_enabled_value(Some("true")));
        assert!(http_enabled_value(None));
    }
}
