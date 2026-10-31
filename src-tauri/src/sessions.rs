use crate::data::{DataStore, LaunchProfile};
use serde_json::{Value, json};
use std::{
    collections::HashMap,
    io::{Read, Write},
    path::PathBuf,
    process::{Child, Command, Stdio},
    sync::{
        Arc, Mutex,
        atomic::{AtomicU64, Ordering},
    },
    thread,
};
use tauri::{AppHandle, Emitter, Manager};

struct Live {
    child: Child,
    input: Arc<Mutex<std::process::ChildStdin>>,
    key_path: Option<PathBuf>,
    ssh: bool,
}

pub struct SessionManager {
    live: Mutex<HashMap<String, Live>>,
    next_id: AtomicU64,
}

impl SessionManager {
    pub fn new() -> Self {
        Self {
            live: Mutex::new(HashMap::new()),
            next_id: AtomicU64::new(1),
        }
    }

    pub fn create(
        &self,
        app: &AppHandle,
        db: &DataStore,
        profile_id: &str,
        name: &str,
        source: &str,
        mode: &str,
        cols: i64,
        rows: i64,
    ) -> Result<Value, String> {
        if mode != "AUTO" && mode != "MANUAL" {
            return Err("mode must be AUTO or MANUAL".into());
        }
        let profile = db.launch_profile(profile_id)?;
        let cols = if cols == 0 { profile.cols } else { cols };
        let rows = if rows == 0 { profile.rows } else { rows };
        if !(20..=500).contains(&cols) || !(5..=200).contains(&rows) {
            return Err("Terminal dimensions are invalid".into());
        }
        let id = format!(
            "rust-session-{}-{}",
            crate::data::session_now(),
            self.next_id.fetch_add(1, Ordering::Relaxed)
        );
        let (mut child, target, key_path, auth_secret, ssh) = spawn_profile(db, &profile, &id)?;
        let auth_secret = auth_secret.map(|secret| Arc::new(Mutex::new(Some(secret))));
        let mut stdout = child.stdout.take().ok_or("无法打开 session 输出")?;
        let mut stderr = child.stderr.take().ok_or("无法打开 session 错误输出")?;
        let input = Arc::new(Mutex::new(
            child.stdin.take().ok_or("无法打开 session 输入")?,
        ));
        db.insert_session(
            &id,
            profile_id,
            &profile.name,
            &target,
            cols,
            rows,
            name.trim(),
            source,
            mode,
        )?;
        let output_app = app.clone();
        let output_id = id.clone();
        let output_input = input.clone();
        let output_secret = auth_secret.clone();
        thread::spawn(move || read_output(&output_app, &output_id, &mut stdout, &output_input, output_secret));
        let error_app = app.clone();
        let error_id = id.clone();
        let error_input = input.clone();
        thread::spawn(move || read_output(&error_app, &error_id, &mut stderr, &error_input, auth_secret));
        self.live
            .lock()
            .unwrap()
            .insert(id.clone(), Live { child, input, key_path, ssh });
        let event = json!({"event":"session.created","data":{"sessionId":id}});
        if app
            .try_state::<std::sync::Arc<crate::bridge::Backend>>()
            .is_none_or(|backend| backend.allows_event(&event))
        {
            let _ = app.emit("shellink:event", event);
        }
        Ok(json!({"id":id,"state":"CONNECTING","mode":mode,"target":target}))
    }

    pub fn list(&self, db: &DataStore) -> Result<Value, String> {
        self.reap_finished(db);
        db.sessions()
    }
    pub fn state(&self, db: &DataStore, id: &str) -> Result<Value, String> {
        self.reap_finished(db);
        db.session_state(id)
    }

    pub fn input(
        &self,
        db: &DataStore,
        id: &str,
        text: &str,
        append: bool,
        manual: bool,
    ) -> Result<Value, String> {
        self.reap_finished(db);
        if db.session_mode(id)? == "MANUAL" && !manual {
            return Err("Session is in MANUAL mode".into());
        }
        let mut live = self.live.lock().unwrap();
        let session = live.get_mut(id).ok_or("Session not found")?;
        let mut input = session.input.lock().map_err(|_| "Session input unavailable")?;
        input
            .write_all(text.as_bytes())
            .map_err(|e| e.to_string())?;
        if append {
            input.write_all(b"\n").map_err(|e| e.to_string())?;
        }
        input.flush().map_err(|e| e.to_string())?;
        let recorded = if append {
            format!("{text}\n")
        } else {
            text.to_owned()
        };
        db.add_input(id, &recorded)?;
        Ok(json!({"ok":true}))
    }

    pub fn set_mode(&self, db: &DataStore, id: &str, mode: &str) -> Result<Value, String> {
        db.update_session_mode(id, mode)
    }
    pub fn resize(&self, db: &DataStore, id: &str, cols: i64, rows: i64) -> Result<Value, String> {
        if !(20..=500).contains(&cols) || !(5..=200).contains(&rows) {
            return Err("Terminal dimensions are invalid".into());
        }
        if let Some(session) = self.live.lock().unwrap().get_mut(id) {
            if session.ssh {
                let mut input = session.input.lock().map_err(|_| "Session input unavailable")?;
                write!(input, "stty cols {cols} rows {rows}\n").map_err(|e| e.to_string())?;
                input.flush().map_err(|e| e.to_string())?;
            }
        }
        db.update_session_size(id, cols, rows)
    }
    pub fn rename(&self, db: &DataStore, id: &str, name: &str) -> Result<Value, String> {
        db.rename_session(id, name)
    }
    pub fn history(
        &self,
        db: &DataStore,
        id: &str,
        since: i64,
        limit: i64,
    ) -> Result<Value, String> {
        self.reap_finished(db);
        if since < 0 {
            return Err("since must be non-negative".into());
        }
        if !(1..=10_000).contains(&limit) {
            return Err("limit must be between 1 and 10000".into());
        }
        db.history_chunks(id, since, limit)
    }
    pub fn replay(&self, db: &DataStore, id: &str) -> Result<Value, String> {
        self.reap_finished(db);
        db.replay(id)
    }
    pub fn close(&self, db: &DataStore, id: &str) -> Result<Value, String> {
        let mut live = self.live.lock().unwrap();
        if let Some(mut session) = live.remove(id) {
            let _ = session.child.kill();
            let _ = session.child.wait();
            cleanup_key(session.key_path.take());
        }
        drop(live);
        db.update_session_state(id, "DISCONNECTED", Some("Closed by user"))?;
        Ok(json!({"closed":true}))
    }
    pub fn remove(&self, db: &DataStore, id: &str) -> Result<Value, String> {
        let _ = self.close(db, id);
        db.remove_session(id)
    }
    pub fn shutdown(&self, db: &DataStore) {
        let ids: Vec<String> = self.live.lock().unwrap().keys().cloned().collect();
        for id in ids {
            let _ = self.close(db, &id);
        }
    }

    fn reap_finished(&self, db: &DataStore) {
        let mut ended = Vec::new();
        let mut live = self.live.lock().unwrap();
        for (id, session) in live.iter_mut() {
            if let Ok(Some(status)) = session.child.try_wait() {
                ended.push((id.clone(), status.code(), session.key_path.clone()));
            }
        }
        for (id, _, _) in &ended {
            live.remove(id);
        }
        drop(live);
        for (id, code, key_path) in ended {
            let _ = db.update_session_state(&id, "DISCONNECTED", Some("Process exited"));
            let _ = db.set_exit_code(&id, code);
            cleanup_key(key_path);
        }
    }
}

fn spawn_profile(
    db: &DataStore,
    profile: &LaunchProfile,
    session_id: &str,
) -> Result<(Child, String, Option<PathBuf>, Option<String>, bool), String> {
    let mut command;
    let target;
    let mut key_path = None;
    let mut auth_secret = None;
    let ssh;
    if profile.connect_type == "command" {
        ssh = false;
        let script = profile.command.clone().unwrap_or_default();
        if script.trim().is_empty() {
            return Err("Command profile has no command".into());
        }
        #[cfg(windows)]
        {
            command = Command::new(std::env::var("COMSPEC").unwrap_or_else(|_| "cmd.exe".into()));
            command.args(["/D", "/S", "/C", &script]);
        }
        #[cfg(not(windows))]
        {
            command = Command::new("/bin/sh");
            command.args(["-lc", &script]);
        }
        target = script;
    } else if profile.connect_type == "ssh" {
        ssh = true;
        command = Command::new(if cfg!(windows) { "ssh.exe" } else { "ssh" });
        let port = profile.port.to_string();
        command.args([
            "-tt",
            "-p",
            &port,
            "-o",
            "ServerAliveInterval=15",
            "-o",
            "ServerAliveCountMax=40",
            "-o",
            "StrictHostKeyChecking=accept-new",
        ]);
        if let Some(encoded) = profile.private_key.as_deref() {
            let key = db.decrypt_secret(encoded)?;
            let path = db.data_home().join(format!(".shellink-key-{session_id}"));
            std::fs::write(&path, key).map_err(|e| format!("写入临时 SSH key 失败: {e}"))?;
            #[cfg(unix)]
            {
                use std::os::unix::fs::PermissionsExt;
                let mut permissions = std::fs::metadata(&path)
                    .map_err(|e| format!("读取临时 SSH key 权限失败: {e}"))?
                    .permissions();
                permissions.set_mode(0o600);
                std::fs::set_permissions(&path, permissions)
                    .map_err(|e| format!("设置临时 SSH key 权限失败: {e}"))?;
            }
            key_path = Some(path.clone());
            if profile.auth_type == "key" {
                auth_secret = profile
                    .passphrase
                    .as_deref()
                    .map(|encoded| db.decrypt_secret(encoded))
                    .transpose()?;
            }
            let path_string = path.to_string_lossy().into_owned();
            command.args([
                "-i",
                path_string.as_str(),
                "-o",
                "IdentitiesOnly=yes",
            ]);
        } else if profile.auth_type == "password" {
            auth_secret = profile
                .password
                .as_deref()
                .map(|encoded| db.decrypt_secret(encoded))
                .transpose()?;
        }
        if profile.host.trim().is_empty() || profile.username.trim().is_empty() {
            return Err("SSH profile requires host and username".into());
        }
        command.arg(format!("{}@{}", profile.username, profile.host));
        target = format!("{}@{}:{}", profile.username, profile.host, profile.port);
    } else {
        return Err("connectType must be ssh or command".into());
    }
    command
        .env("TERM", &profile.term)
        .stdin(Stdio::piped())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped());
    command
        .spawn()
        .map(|child| (child, target, key_path, auth_secret, ssh))
        .map_err(|e| format!("启动 session 失败: {e}"))
}

fn read_output(
    app: &AppHandle,
    id: &str,
    stream: &mut impl Read,
    input: &Arc<Mutex<std::process::ChildStdin>>,
    auth_secret: Option<Arc<Mutex<Option<String>>>>,
) {
    let mut buf = [0u8; 8192];
    loop {
        match stream.read(&mut buf) {
            Ok(0) | Err(_) => break,
            Ok(n) => {
                maybe_answer_auth(input, auth_secret.as_ref(), &buf[..n]);
                emit_output(app, id, &buf[..n]);
            }
        }
    }
}

fn emit_output(app: &AppHandle, id: &str, bytes: &[u8]) {
    let text = String::from_utf8_lossy(bytes).into_owned();
    let db = app.state::<DataStore>();
    let _ = db.add_output(id, &text);
    let _ = db.update_session_state(id, "OUTPUTTING", None);
    let event = json!({"event":"session.output","data":{"sessionId":id,"direction":"output","raw":text,"plain":text}});
    if app
        .try_state::<std::sync::Arc<crate::bridge::Backend>>()
        .is_none_or(|backend| backend.allows_event(&event))
    {
        let _ = app.emit("shellink:event", event);
    }
}

fn maybe_answer_auth(
    input: &Arc<Mutex<std::process::ChildStdin>>,
    auth_secret: Option<&Arc<Mutex<Option<String>>>>,
    bytes: &[u8],
) {
    let Some(auth_secret) = auth_secret else { return };
    let text = String::from_utf8_lossy(bytes).to_ascii_lowercase();
    if !(text.contains("password:") || text.contains("passphrase") || text.contains("password for")) {
        return;
    }
    let Ok(mut secret) = auth_secret.lock() else { return };
    let Some(value) = secret.take() else { return };
    drop(secret);
    if let Ok(mut input) = input.lock() {
        let _ = input.write_all(value.as_bytes());
        let _ = input.write_all(b"\n");
        let _ = input.flush();
    }
}

fn cleanup_key(path: Option<PathBuf>) {
    if let Some(path) = path {
        let _ = std::fs::remove_file(path);
    }
}
