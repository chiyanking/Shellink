use crate::data::{DataStore, LaunchProfile};
use crate::ssh_auth::{Credential, SshAuth};
use portable_pty::{Child, CommandBuilder, MasterPty, PtySize, native_pty_system};
use serde_json::{Value, json};
use std::{
    collections::HashMap,
    io::{Read, Write},
    path::PathBuf,
    sync::{
        Arc, Mutex,
        atomic::{AtomicBool, AtomicU64, Ordering},
    },
    thread,
};
use tauri::{AppHandle, Emitter, Manager};

type SessionInput = Arc<Mutex<Box<dyn Write + Send>>>;

struct Live {
    child: Box<dyn Child + Send + Sync>,
    input: SessionInput,
    pty: Box<dyn MasterPty + Send>,
    output_finished: Arc<AtomicBool>,
    key_path: Option<PathBuf>,
    auth: Option<SshAuth>,
}

struct SpawnedSession {
    live: Live,
    output: Box<dyn Read + Send>,
    target: String,
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
        let SpawnedSession {
            mut live,
            mut output,
            target,
        } = spawn_profile(app, db, &profile, &id, cols, rows)?;
        if let Err(error) = db.insert_session(
            &id,
            profile_id,
            &profile.name,
            &target,
            cols,
            rows,
            name.trim(),
            source,
            mode,
        ) {
            let _ = live.child.kill();
            let _ = live.child.wait();
            cleanup_key(live.key_path.take());
            return Err(error);
        }
        let output_app = app.clone();
        let output_id = id.clone();
        let output_finished = live.output_finished.clone();
        self.live.lock().unwrap().insert(id.clone(), live);
        thread::spawn(move || {
            read_output(&output_app, &output_id, &mut output);
            output_finished.store(true, Ordering::Release);
        });
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
        if session
            .auth
            .as_ref()
            .is_some_and(|auth| auth.prompt().is_some())
        {
            return Err("请先在 SSH 认证窗口中完成 MFA/2FA 验证".into());
        }
        let mut input = session
            .input
            .lock()
            .map_err(|_| "Session input unavailable")?;
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

    pub fn auth_pending(&self, db: &DataStore) -> Result<Value, String> {
        self.reap_finished(db);
        let prompts: Vec<Value> = self
            .live
            .lock()
            .unwrap()
            .iter()
            .filter_map(|(id, session)| {
                let mut prompt = session.auth.as_ref()?.prompt()?;
                prompt["sessionId"] = json!(id);
                Some(prompt)
            })
            .collect();
        Ok(json!(prompts))
    }

    pub fn auth_respond(
        &self,
        db: &DataStore,
        id: &str,
        prompt_id: u64,
        response: &str,
    ) -> Result<Value, String> {
        self.reap_finished(db);
        let live = self.live.lock().unwrap();
        let auth = live
            .get(id)
            .and_then(|session| session.auth.as_ref())
            .ok_or("SSH 认证请求已结束")?;
        auth.respond(prompt_id, response)?;
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
            session
                .pty
                .resize(terminal_size(cols, rows))
                .map_err(|e| e.to_string())?;
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
            if session.output_finished.load(Ordering::Acquire) {
                if let Ok(Some(status)) = session.child.try_wait() {
                    ended.push((id.clone(), status.exit_code(), session.key_path.clone()));
                }
            }
        }
        for (id, _, _) in &ended {
            live.remove(id);
        }
        drop(live);
        for (id, code, key_path) in ended {
            let _ = db.update_session_state(&id, "DISCONNECTED", Some("Process exited"));
            let _ = db.set_exit_code(&id, Some(code as i32));
            cleanup_key(key_path);
        }
    }
}

fn spawn_profile(
    app: &AppHandle,
    db: &DataStore,
    profile: &LaunchProfile,
    session_id: &str,
    cols: i64,
    rows: i64,
) -> Result<SpawnedSession, String> {
    let mut command;
    let target;
    let mut key_path = None;
    let mut auth = None;
    if profile.connect_type == "command" {
        let script = profile.command.clone().unwrap_or_default();
        if script.trim().is_empty() {
            return Err("Command profile has no command".into());
        }
        #[cfg(windows)]
        {
            command =
                CommandBuilder::new(std::env::var("COMSPEC").unwrap_or_else(|_| "cmd.exe".into()));
            command.args(["/D", "/S", "/C", &script]);
        }
        #[cfg(not(windows))]
        {
            command = CommandBuilder::new("/bin/sh");
            command.args(["-lc", &script]);
        }
        target = script;
    } else if profile.connect_type == "ssh" {
        command = CommandBuilder::new(if cfg!(windows) { "ssh.exe" } else { "ssh" });
        if profile.host.trim().is_empty() || profile.username.trim().is_empty() {
            return Err("SSH profile requires host and username".into());
        }
        if !matches!(
            profile.auth_type.as_str(),
            "password" | "password-mfa" | "key" | "keyboard-interactive"
        ) {
            return Err("Unsupported SSH authentication type".into());
        }
        let saved_secret = match profile.auth_type.as_str() {
            "password" | "password-mfa" => profile.password.as_deref(),
            "key" => profile.passphrase.as_deref(),
            _ => None,
        }
        .map(|encoded| db.decrypt_secret(encoded))
        .transpose()?;
        let auth_app = app.clone();
        let auth_id = session_id.to_owned();
        let ssh_auth = SshAuth::start(
            saved_secret.map(|secret| Credential {
                secret,
                is_key: profile.auth_type == "key",
            }),
            move || {
                let _ = auth_app.emit(
                    "shellink:event",
                    json!({"event":"session.auth","data":{"sessionId":auth_id}}),
                );
            },
        )?;
        let executable = std::env::current_exe().map_err(|e| e.to_string())?;
        command.env("SSH_ASKPASS", executable);
        command.env("SSH_ASKPASS_REQUIRE", "force");
        command.env(
            "DISPLAY",
            std::env::var("DISPLAY").unwrap_or_else(|_| "shellink:0".into()),
        );
        command.env("SHELLINK_PROCESS_MODE", "ssh-askpass");
        command.env("SHELLINK_ASKPASS_ADDR", &ssh_auth.address);
        command.env("SHELLINK_ASKPASS_TOKEN", &ssh_auth.token);
        auth = Some(ssh_auth);
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
            "-o",
            "BatchMode=no",
            "-o",
            "KbdInteractiveAuthentication=yes",
            "-o",
            "PasswordAuthentication=yes",
        ]);
        let methods = match profile.auth_type.as_str() {
            "key" => "PreferredAuthentications=publickey,keyboard-interactive,password",
            "keyboard-interactive" => {
                "PreferredAuthentications=keyboard-interactive,publickey,password"
            }
            _ => "PreferredAuthentications=password,keyboard-interactive,publickey",
        };
        command.args(["-o", methods]);
        if let Some(encoded) = profile
            .private_key
            .as_deref()
            .filter(|_| profile.auth_type == "key")
        {
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
            let path_string = path.to_string_lossy().into_owned();
            command.args(["-i", path_string.as_str(), "-o", "IdentitiesOnly=yes"]);
        }
        command.arg(format!("{}@{}", profile.username, profile.host));
        target = format!("{}@{}:{}", profile.username, profile.host, profile.port);
    } else {
        return Err("connectType must be ssh or command".into());
    }
    command.env("TERM", &profile.term);
    match spawn_terminal(command, cols, rows) {
        Ok((live, output)) => Ok(SpawnedSession {
            live: Live {
                key_path,
                auth,
                ..live
            },
            output,
            target,
        }),
        Err(error) => {
            cleanup_key(key_path);
            Err(error)
        }
    }
}

fn terminal_size(cols: i64, rows: i64) -> PtySize {
    PtySize {
        cols: cols as u16,
        rows: rows as u16,
        pixel_width: 0,
        pixel_height: 0,
    }
}

fn spawn_terminal(
    command: CommandBuilder,
    cols: i64,
    rows: i64,
) -> Result<(Live, Box<dyn Read + Send>), String> {
    let pair = native_pty_system()
        .openpty(terminal_size(cols, rows))
        .map_err(|e| format!("打开终端失败: {e}"))?;
    let output = pair.master.try_clone_reader().map_err(|e| e.to_string())?;
    let input = pair.master.take_writer().map_err(|e| e.to_string())?;
    let child = pair
        .slave
        .spawn_command(command)
        .map_err(|e| format!("启动 session 失败: {e}"))?;
    drop(pair.slave);
    Ok((
        Live {
            child,
            input: Arc::new(Mutex::new(input)),
            pty: pair.master,
            output_finished: Arc::new(AtomicBool::new(false)),
            key_path: None,
            auth: None,
        },
        output,
    ))
}

fn read_output(app: &AppHandle, id: &str, stream: &mut impl Read) {
    let mut buf = [0u8; 8192];
    let mut pending = Vec::new();
    loop {
        match stream.read(&mut buf) {
            Ok(0) | Err(_) => break,
            Ok(n) => {
                pending.extend_from_slice(&buf[..n]);
                let valid = match std::str::from_utf8(&pending) {
                    Ok(_) => pending.len(),
                    Err(error) if error.error_len().is_none() => error.valid_up_to(),
                    Err(_) => pending.len(),
                };
                if valid == 0 {
                    continue;
                }
                let text = String::from_utf8_lossy(&pending[..valid]).into_owned();
                pending.drain(..valid);
                emit_output(app, id, &text);
            }
        }
    }
    if !pending.is_empty() {
        emit_output(app, id, &String::from_utf8_lossy(&pending));
    }
}

fn emit_output(app: &AppHandle, id: &str, text: &str) {
    let db = app.state::<DataStore>();
    let Ok(seq) = db.add_output(id, text) else {
        return;
    };
    let event = json!({"event":"session.data","seq":seq,"data":{"sessionId":id,"direction":"output","raw":text,"plain":text,"seq":seq}});
    if app
        .try_state::<Arc<crate::bridge::Backend>>()
        .is_none_or(|backend| backend.allows_event(&event))
    {
        let _ = app.emit("shellink:event", event);
    }
}

fn cleanup_key(path: Option<PathBuf>) {
    if let Some(path) = path {
        let _ = std::fs::remove_file(path);
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[cfg(unix)]
    #[test]
    fn terminal_supports_tty_authentication_and_native_resize() {
        use std::{sync::mpsc, time::Duration};
        let mut command = CommandBuilder::new("/bin/sh");
        command.args([
            "-c",
            r#"
            test -t 0 && test -t 1 || exit 2
            stty -echo
            printf 'Password: ' > /dev/tty
            IFS= read -r answer < /dev/tty
            test "$answer" = 'test-secret' || exit 3
            printf '\nAUTH_OK\n'
            stty size
            exit 7
        "#,
        ]);
        let (mut live, mut output) = spawn_terminal(command, 120, 36).unwrap();
        live.pty.resize(terminal_size(100, 30)).unwrap();
        let input = live.input.clone();
        let (sender, receiver) = mpsc::channel();
        let reader = thread::spawn(move || {
            let mut answered = false;
            let mut all = String::new();
            let mut bytes = [0u8; 1024];
            while let Ok(n) = output.read(&mut bytes) {
                if n == 0 {
                    break;
                }
                let text = String::from_utf8_lossy(&bytes[..n]);
                all.push_str(&text);
                if !answered && all.contains("Password: ") {
                    let mut writer = input.lock().unwrap();
                    writer.write_all(b"test-secret\n").unwrap();
                    writer.flush().unwrap();
                    answered = true;
                }
            }
            let _ = sender.send(all);
        });
        let result = receiver.recv_timeout(Duration::from_secs(5));
        if result.is_err() {
            let _ = live.child.kill();
        }
        let status = live.child.wait().unwrap();
        reader.join().unwrap();
        let output = result.expect("PTY authentication timed out");
        assert_eq!(status.exit_code(), 7, "{output}");
        assert!(output.contains("AUTH_OK"), "{output}");
        assert!(output.contains("30 100"), "{output}");
        assert!(
            !output.contains("test-secret"),
            "secret was echoed by terminal"
        );
    }
}
