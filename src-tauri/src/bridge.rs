use crate::{data::DataStore, mcp::Server, sessions::SessionManager};
use serde_json::{Value, json};
use std::{
    collections::HashMap,
    env,
    path::PathBuf,
    process::Command,
    sync::{
        Arc, Mutex,
        atomic::{AtomicU64, Ordering},
    },
    time::{Instant, SystemTime, UNIX_EPOCH},
};
use tauri::{AppHandle, Manager};

pub type Reply = Result<Value, String>;

pub struct Backend {
    next_subscription: AtomicU64,
    subscriptions: Mutex<HashMap<String, Subscription>>,
    started: Instant,
}

struct Subscription {
    session_id: Option<String>,
    replay: bool,
}

impl Default for Backend {
    fn default() -> Self {
        Self {
            next_subscription: AtomicU64::new(0),
            subscriptions: Mutex::new(HashMap::new()),
            started: Instant::now(),
        }
    }
}

impl Backend {
    fn subscribe(&self, params: &Value) -> Result<Value, String> {
        let id = format!(
            "desktop-subscription-{}",
            self.next_subscription.fetch_add(1, Ordering::Relaxed) + 1
        );
        let session_id = match params.get("sessionId") {
            None | Some(Value::Null) => None,
            Some(value) => Some(
                value
                    .as_str()
                    .filter(|value| !value.is_empty())
                    .ok_or("sessionId must be a non-empty string")?
                    .to_owned(),
            ),
        };
        let replay = params
            .get("replay")
            .and_then(Value::as_bool)
            .unwrap_or(false);
        self.subscriptions
            .lock()
            .unwrap()
            .insert(id.clone(), Subscription { session_id, replay });
        Ok(json!({"subscriptionId": id, "replay": replay}))
    }

    fn unsubscribe(&self, id: &str) -> Value {
        json!({"removed": self.subscriptions.lock().unwrap().remove(id).is_some()})
    }

    pub fn allows_event(&self, event: &Value) -> bool {
        let session_id = event
            .get("data")
            .and_then(|data| data.get("sessionId"))
            .and_then(Value::as_str);
        self.subscriptions.lock().unwrap().values().any(|subscription| {
            let replay = subscription.replay;
            let session_matches = subscription
                .session_id
                .as_deref()
                .is_none_or(|wanted| Some(wanted) == session_id);
            session_matches && (replay || event.get("event").is_some())
        })
    }

    pub fn shutdown(&self, _app: &AppHandle) {
        self.subscriptions.lock().unwrap().clear();
    }
}

fn required_string<'a>(params: &'a Value, key: &str) -> Result<&'a str, String> {
    params
        .get(key)
        .and_then(Value::as_str)
        .filter(|value| !value.is_empty())
        .ok_or_else(|| format!("{key} must be a non-empty string"))
}

fn optional_i64(params: &Value, key: &str, default: i64) -> Result<i64, String> {
    match params.get(key) {
        None | Some(Value::Null) => Ok(default),
        Some(value) => value
            .as_i64()
            .ok_or_else(|| format!("{key} must be an integer")),
    }
}

fn system_status(
    app: &AppHandle,
    db: &DataStore,
    manager: &SessionManager,
    server: &Server,
    backend: &Backend,
) -> Reply {
    let sessions = manager.list(db)?;
    let active_sessions = sessions
        .as_array()
        .map(|items| {
            items
                .iter()
                .filter(|item| item.get("active").and_then(Value::as_bool).unwrap_or(false))
                .count()
        })
        .unwrap_or(0);
    let mcp = server.info(db)?;
    let http_enabled = mcp.get("running").and_then(Value::as_bool).unwrap_or(false);
    let http_url = if http_enabled {
        mcp.get("url")
            .and_then(Value::as_str)
            .map(|url| url.trim_end_matches("/shellink/mcp").to_owned())
    } else {
        None
    };
    Ok(json!({
        "version": env!("CARGO_PKG_VERSION"),
        "commit": "unknown",
        "protocolVersion": 1,
        "pid": std::process::id(),
        "uptimeSeconds": backend.started.elapsed().as_secs(),
        "socketPath": Value::Null,
        "httpEnabled": http_enabled,
        "httpUrl": http_url,
        "activeSessions": active_sessions,
        "app": app.package_info().version.to_string(),
    }))
}

fn desktop_info(
    app: &AppHandle,
    db: &DataStore,
    manager: &SessionManager,
    server: &Server,
    backend: &Backend,
) -> Reply {
    let command = std::env::current_exe()
        .map(|path| path.to_string_lossy().into_owned())
        .unwrap_or_default();
    let shell = if cfg!(windows) {
        "powershell.exe".to_owned()
    } else {
        env::var("SHELL").unwrap_or_else(|_| "/bin/sh".into())
    };
    let status = system_status(app, db, manager, server, backend)?;
    Ok(json!({
        "status": status,
        "dataHome": db.data_home().to_string_lossy(),
        "logPath": db.data_home().join("shellink.log").to_string_lossy(),
        "command": command,
        "defaultShell": shell,
        "platform": std::env::consts::OS,
    }))
}

fn dispatch(app: &AppHandle, backend: &Backend, method: &str, params: &Value) -> Reply {
    let db = app.state::<DataStore>();
    let manager = app.state::<Arc<SessionManager>>();
    let server = app.state::<Server>();
    match method {
        "desktop.info" => desktop_info(app, &db, &manager, &server, backend),
        "system.hello" => Ok(json!({
            "serviceVersion": env!("CARGO_PKG_VERSION"),
            "serviceCommit": "unknown",
            "protocolVersion": 1,
            "capabilities": ["system.hello", "system.ping", "system.status", "system.stop", "profiles.list", "profiles.get", "profiles.create", "profiles.update", "profiles.delete", "sessions.list", "sessions.create", "sessions.state", "sessions.history", "sessions.input", "sessions.mode", "sessions.resize", "sessions.close", "sessions.removeRecord", "sessions.rename", "sessions.replay", "sessions.historyChunks", "webhooks.list", "webhooks.create", "webhooks.delete", "webhooks.messages", "webhooks.clearMessages", "events.subscribe", "events.unsubscribe", "mcp.info", "mcp.config"]
        })),
        "system.ping" => Ok(json!({"pong": true, "at": now()})),
        "system.status" => system_status(app, &db, &manager, &server, backend),
        "system.stop" => {
            server.stop();
            manager.shutdown(&db);
            Ok(json!({"stopping": true}))
        }
        "mcp.info" => server.info(&db),
        "mcp.config" => server.update_config(app, &db, params),
        "profiles.list" => db.profiles(None),
        "profiles.get" => db.profiles(Some(required_string(params, "id")?)),
        "profiles.create" => db.profile_create(params),
        "profiles.update" => db.profile_update(
            required_string(params, "id")?,
            params.get("profile").ok_or("profile is required")?,
        ),
        "profiles.delete" => db.profile_delete(required_string(params, "id")?),
        "sessions.list" => manager.list(&db),
        "sessions.create" => {
            let mode = params.get("mode").and_then(Value::as_str).unwrap_or("AUTO");
            let source = params
                .get("source")
                .and_then(Value::as_str)
                .unwrap_or("unknown");
            let name = params
                .get("name")
                .and_then(Value::as_str)
                .unwrap_or("")
                .trim();
            if name.chars().count() > 80 {
                return Err("name must be at most 80 characters".into());
            }
            manager.create(
                app,
                &db,
                required_string(params, "profileId")?,
                name,
                source,
                mode,
                optional_i64(params, "cols", 0)?,
                optional_i64(params, "rows", 0)?,
            )
        }
        "sessions.state" => manager.state(&db, required_string(params, "id")?),
        "sessions.history" => manager.history(
            &db,
            required_string(params, "id")?,
            optional_i64(params, "since", 0)?,
            optional_i64(params, "limit", 2000)?,
        ),
        "sessions.historyChunks" => manager.history(
            &db,
            required_string(params, "id")?,
            optional_i64(params, "since", 0)?,
            optional_i64(params, "limit", 500)?,
        ),
        "sessions.input" => manager.input(
            &db,
            required_string(params, "id")?,
            params
                .get("text")
                .and_then(Value::as_str)
                .ok_or("text must be a string")?,
            params
                .get("appendNewline")
                .and_then(Value::as_bool)
                .unwrap_or(true),
            params
                .get("manual")
                .and_then(Value::as_bool)
                .unwrap_or(false),
        ),
        "sessions.mode" => manager.set_mode(
            &db,
            required_string(params, "id")?,
            required_string(params, "mode")?,
        ),
        "sessions.resize" => manager.resize(
            &db,
            required_string(params, "id")?,
            optional_i64(params, "cols", 0)?,
            optional_i64(params, "rows", 0)?,
        ),
        "sessions.close" => manager.close(&db, required_string(params, "id")?),
        "sessions.removeRecord" => manager.remove(&db, required_string(params, "id")?),
        "sessions.rename" => manager.rename(
            &db,
            required_string(params, "id")?,
            required_string(params, "name")?,
        ),
        "sessions.replay" => manager.replay(&db, required_string(params, "id")?),
        "webhooks.list" => db.webhooks(),
        "webhooks.create" => db.webhook_create(params),
        "webhooks.delete" => db.webhook_delete(required_string(params, "id")?),
        "webhooks.messages" => Ok(server.messages()),
        "webhooks.clearMessages" => {
            server.clear_messages();
            Ok(json!({"ok": true}))
        }
        "events.subscribe" => backend.subscribe(params),
        "events.unsubscribe" => Ok(backend.unsubscribe(required_string(params, "subscriptionId")?)),
        _ => Err(format!("Unknown method: {method}")),
    }
}

fn now() -> i64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap_or_default()
        .as_millis() as i64
}

#[tauri::command]
pub async fn core_request(
    app: AppHandle,
    state: tauri::State<'_, Arc<Backend>>,
    method: String,
    params: Value,
    timeout_ms: Option<u64>,
) -> Reply {
    let backend = state.inner().clone();
    tauri::async_runtime::spawn_blocking(move || {
        let _ = timeout_ms;
        dispatch(&app, &backend, &method, &params)
    })
    .await
    .map_err(|error| error.to_string())?
}

#[tauri::command]
pub fn open_data_directory(path: String) -> Result<(), String> {
    let path = PathBuf::from(path);
    if !path.is_dir() {
        return Err("目录不存在".into());
    }
    #[cfg(target_os = "macos")]
    let result = Command::new("open").arg(path).spawn();
    #[cfg(target_os = "windows")]
    let result = Command::new("explorer.exe").arg(path).spawn();
    #[cfg(target_os = "linux")]
    let result = Command::new("xdg-open").arg(path).spawn();
    result.map(|_| ()).map_err(|error| error.to_string())
}
