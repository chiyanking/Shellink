use serde_json::{json, Value};
use std::{collections::HashMap, io::{BufRead, BufReader, Write}, path::PathBuf, process::{Child, ChildStdin, Command, Stdio}, sync::{Arc, Mutex, atomic::{AtomicBool, AtomicU64, Ordering}, mpsc}, time::Duration};
use tauri::{AppHandle, Emitter, Manager};

type Reply = Result<Value, String>;
struct Transport {
    child: Mutex<Child>, input: Mutex<ChildStdin>,
    pending: Mutex<HashMap<u64, mpsc::Sender<Reply>>>, diagnostics: Mutex<String>,
    alive: AtomicBool, retired: AtomicBool,
}
impl Transport {
    fn stop(&self) { self.retired.store(true, Ordering::SeqCst); self.alive.store(false, Ordering::SeqCst); let _ = self.child.lock().unwrap().kill(); let _ = self.child.lock().unwrap().wait(); }
}
#[derive(Default)]
pub struct Backend { transport: Mutex<Option<Arc<Transport>>>, next: AtomicU64 }
impl Backend {
    fn core_path(app: &AppHandle) -> Result<PathBuf, String> {
        let suffix = if cfg!(windows) { ".exe" } else { "" };
        #[cfg(debug_assertions)]
        if let Some(path) = std::env::var_os("SHELLINK_CORE") { return Ok(path.into()); }
        let mut candidates = vec![];
        if let Ok(exe) = std::env::current_exe() { if let Some(dir) = exe.parent() { candidates.push(dir.join(format!("shellink-core{suffix}"))); } }
        if let Ok(dir) = app.path().resource_dir() { candidates.push(dir.join(format!("shellink-core{suffix}"))); }
        #[cfg(debug_assertions)]
        candidates.push(PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("binaries").join(format!("shellink-core-{}{suffix}", env!("SHELLINK_TARGET"))));
        candidates.into_iter().find(|path| path.is_file()).ok_or_else(|| "未找到随包 Shellink 核心，请重新构建或安装应用。".into())
    }
    fn connect(&self, app: &AppHandle) -> Result<Arc<Transport>, String> {
        let mut current = self.transport.lock().unwrap();
        if let Some(transport) = current.as_ref() {
            if transport.alive.load(Ordering::SeqCst) && matches!(transport.child.lock().unwrap().try_wait(), Ok(None)) { return Ok(transport.clone()); }
            let reason = transport.diagnostics.lock().unwrap().clone();
            return Err(if reason.trim().is_empty() { "与后台的连接已断开，请重新连接。".into() } else { reason });
        }
        let mut command = Command::new(Self::core_path(app)?);
        command.arg("desktop-bridge").stdin(Stdio::piped()).stdout(Stdio::piped()).stderr(Stdio::piped());
        #[cfg(windows)] { use std::os::windows::process::CommandExt; command.creation_flags(0x08000000); }
        let mut child = command.spawn().map_err(|e| format!("无法启动桌面桥接: {e}"))?;
        let input = child.stdin.take().ok_or("无法打开桥接输入")?;
        let output = child.stdout.take().ok_or("无法打开桥接输出")?;
        let diagnostics = child.stderr.take().ok_or("无法打开桥接诊断")?;
        let transport = Arc::new(Transport { child: Mutex::new(child), input: Mutex::new(input), pending: Mutex::new(HashMap::new()), diagnostics: Mutex::new(String::new()), alive: AtomicBool::new(true), retired: AtomicBool::new(false) });
        let log = transport.clone();
        std::thread::spawn(move || { for line in BufReader::new(diagnostics).lines().map_while(Result::ok) { let mut text = log.diagnostics.lock().unwrap(); if text.len() > 8192 { text.clear(); } text.push_str(&line); text.push('\n'); } });
        let reader = transport.clone(); let app = app.clone();
        std::thread::spawn(move || {
            for line in BufReader::new(output).lines() {
                let Ok(line) = line else { break }; if line.len() > 32 << 20 { break; }
                let Ok(value) = serde_json::from_str::<Value>(&line) else { break };
                if let Some(event) = value.get("event") { if !reader.retired.load(Ordering::SeqCst) { let _ = app.emit("shellink:event", event); } continue; }
                if let Some(id) = value.get("id").and_then(Value::as_u64) {
                    if let Some(reply) = reader.pending.lock().unwrap().remove(&id) {
                        let result = match value.get("error") { Some(error) => Err(error.get("message").and_then(Value::as_str).unwrap_or("后台请求失败").to_owned()), None => Ok(value.get("result").cloned().unwrap_or(Value::Null)) };
                        let _ = reply.send(result);
                    }
                }
            }
            reader.alive.store(false, Ordering::SeqCst);
            let diagnostic = reader.diagnostics.lock().unwrap().clone();
            let reason = if diagnostic.trim().is_empty() { "与后台的连接已断开，请重新连接。".to_owned() } else { diagnostic };
            for (_, reply) in reader.pending.lock().unwrap().drain() { let _ = reply.send(Err(reason.clone())); }
            if !reader.retired.load(Ordering::SeqCst) { let _ = app.emit("shellink:disconnected", reason); }
        });
        *current = Some(transport.clone()); Ok(transport)
    }
    pub fn stop(&self) { if let Some(transport) = self.transport.lock().unwrap().take() { transport.stop(); } }
    pub fn shutdown(&self, app: &AppHandle) {
        // Desktop owns the runtime lifetime. Ask the current core to close its
        // HTTP listener and all sessions before killing the bridge process.
        // Do not call request() here: it would lazily start a runtime during
        // application exit when the desktop never used the backend.
        let existing = self.transport.lock().unwrap().clone();
        if let Some(transport) = existing {
            let id = self.next.fetch_add(1, Ordering::Relaxed) + 1;
            let (reply, response) = mpsc::channel();
            transport.pending.lock().unwrap().insert(id, reply);
            let request = json!({ "id": id, "method": "system.stop", "params": {}, "timeoutMs": 2000 });
            let written = { let mut input = transport.input.lock().unwrap(); writeln!(input, "{request}").and_then(|_| input.flush()) };
            if written.is_ok() { let _ = response.recv_timeout(Duration::from_secs(2)); }
            transport.pending.lock().unwrap().remove(&id);
            let _ = app;
        }
        self.stop();
    }
    pub fn request(&self, app: &AppHandle, method: String, params: Value, timeout_ms: u64) -> Reply {
        let transport = self.connect(app)?;
        let id = self.next.fetch_add(1, Ordering::Relaxed) + 1;
        let timeout_ms = timeout_ms.clamp(1000, 180_000);
        let (reply, response) = mpsc::channel(); transport.pending.lock().unwrap().insert(id, reply);
        let request = json!({ "id": id, "method": method, "params": params, "timeoutMs": timeout_ms });
        let written = { let mut input = transport.input.lock().unwrap(); writeln!(input, "{request}").and_then(|_| input.flush()) };
        if let Err(error) = written { transport.pending.lock().unwrap().remove(&id); transport.stop(); return Err(format!("桥接写入失败: {error}")); }
        let result = response.recv_timeout(Duration::from_millis(timeout_ms + 16_000)).map_err(|_| "后台请求超时，请检查服务日志。".to_owned());
        transport.pending.lock().unwrap().remove(&id);
        result?
    }
}

#[tauri::command]
pub async fn core_request(app: AppHandle, state: tauri::State<'_, Arc<Backend>>, method: String, params: Value, timeout_ms: Option<u64>) -> Reply {
    if method == "sessions.state" {
        if let Some(db)=app.try_state::<Option<crate::data::DataStore>>() { if let Some(db)=db.inner().as_ref() { if let Some(id)=params.get("id").and_then(Value::as_str) { if let Ok(value)=db.session_state(id) { return Ok(value); } } } }
    }
    if matches!(method.as_str(), "sessions.create" | "sessions.input" | "sessions.close") {
        if let (Some(db), Some(manager)) = (app.try_state::<Option<crate::data::DataStore>>(), app.try_state::<Arc<crate::sessions::SessionManager>>()) {
            if let Some(db)=db.inner().as_ref() {
                let local=match method.as_str() {
                    "sessions.create" => manager.create(&app,db,params.get("profileId").and_then(Value::as_str).ok_or("profileId is required")?,params.get("name").and_then(Value::as_str).unwrap_or(""),params.get("source").and_then(Value::as_str).unwrap_or("desktop"),params.get("mode").and_then(Value::as_str).unwrap_or("MANUAL"),params.get("cols").and_then(Value::as_i64).unwrap_or(0),params.get("rows").and_then(Value::as_i64).unwrap_or(0)),
                    "sessions.input" => manager.input(params.get("id").and_then(Value::as_str).ok_or("id is required")?,params.get("text").and_then(Value::as_str).ok_or("text is required")?,params.get("appendNewline").and_then(Value::as_bool).unwrap_or(true)),
                    _ => manager.close(db,params.get("id").and_then(Value::as_str).ok_or("id is required")?),
                }; if local.is_ok(){return local;}
            }
        }
    }
    if matches!(method.as_str(), "profiles.list" | "profiles.get" | "profiles.create" | "profiles.update" | "profiles.delete" | "sessions.list") {
        if let Some(store) = app.try_state::<Option<crate::data::DataStore>>() {
            if let Some(store) = store.inner().as_ref() {
                let local = match method.as_str() {
                    "profiles.list" => store.profiles(None),
                    "profiles.get" => params.get("id").and_then(Value::as_str).map_or_else(|| Err("id must be a string".into()), |id| store.profiles(Some(id))),
                    "profiles.create" => store.profile_create(&params),
                    "profiles.update" => params.get("id").and_then(Value::as_str).zip(params.get("profile")).map_or_else(|| Err("id and profile are required".into()), |(id,p)| store.profile_update(id,p)),
                    "profiles.delete" => params.get("id").and_then(Value::as_str).map_or_else(|| Err("id must be a string".into()), |id| store.profile_delete(id)),
                    _ => store.sessions(),
                };
                if local.is_ok() { return local; }
            }
        }
    }
    let backend = state.inner().clone();
    tauri::async_runtime::spawn_blocking(move || backend.request(&app, method, params, timeout_ms.unwrap_or(30_000))).await.map_err(|e| e.to_string())?
}
#[tauri::command]
pub fn core_reconnect(state: tauri::State<'_, Arc<Backend>>) { state.stop(); }
#[tauri::command]
pub fn open_data_directory(path: String) -> Result<(), String> {
    // The frontend obtains this directory from desktop.info; only directories
    // can be opened, and arguments never pass through a shell.
    let path = PathBuf::from(path); if !path.is_dir() { return Err("目录不存在".into()); }
    #[cfg(target_os = "macos")] let result = Command::new("open").arg(path).spawn();
    #[cfg(target_os = "windows")] let result = Command::new("explorer.exe").arg(path).spawn();
    #[cfg(target_os = "linux")] let result = Command::new("xdg-open").arg(path).spawn();
    result.map(|_| ()).map_err(|e| e.to_string())
}
