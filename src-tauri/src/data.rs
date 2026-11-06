use aes_gcm::{
    Aes256Gcm, KeyInit, Nonce,
    aead::{Aead, OsRng, rand_core::RngCore},
};
use base64::{Engine as _, engine::general_purpose::STANDARD};
use rusqlite::{Connection, OptionalExtension};
use serde_json::{Value, json};
use std::{
    fs,
    path::{Path, PathBuf},
    sync::Mutex,
};

pub struct DataStore {
    conn: Mutex<Connection>,
    key: [u8; 32],
    data_home: PathBuf,
}

pub struct LaunchProfile {
    pub name: String,
    pub connect_type: String,
    pub command: Option<String>,
    pub host: String,
    pub port: i64,
    pub username: String,
    pub auth_type: String,
    pub password: Option<String>,
    pub private_key: Option<String>,
    pub passphrase: Option<String>,
    pub term: String,
    pub cols: i64,
    pub rows: i64,
}

impl DataStore {
    #[cfg(test)]
    pub(crate) fn test_store(data_home: PathBuf) -> Self {
        let conn = Connection::open_in_memory().unwrap();
        initialize_schema(&conn).unwrap();
        Self {
            conn: Mutex::new(conn),
            key: [0; 32],
            data_home,
        }
    }

    pub fn open() -> Result<Self, String> {
        let data_home = resolve_data_home()?;
        ensure_private_dir(&data_home)?;
        let path = std::env::var_os("SHELLINK_DB")
            .filter(|value| !value.is_empty())
            .map(PathBuf::from)
            .map(make_absolute)
            .transpose()?
            .unwrap_or_else(|| data_home.join("shellink.db"));
        if let Some(parent) = path.parent() {
            ensure_private_dir(parent)?;
        }
        let conn = Connection::open(&path).map_err(|e| format!("打开数据库失败: {e}"))?;
        conn.busy_timeout(std::time::Duration::from_secs(5))
            .map_err(|e| e.to_string())?;
        conn.execute_batch(
            "PRAGMA journal_mode=WAL; PRAGMA synchronous=NORMAL; PRAGMA foreign_keys=ON; PRAGMA busy_timeout=5000;",
        )
        .map_err(|e| e.to_string())?;
        initialize_schema(&conn)?;
        migrate_legacy_mcp_json(&conn, &data_home)?;
        let key = load_master_key(&data_home, std::env::var("SHELLINK_MASTER_KEY").ok())?;
        Ok(Self {
            conn: Mutex::new(conn),
            key,
            data_home,
        })
    }
    pub fn profiles(&self, id: Option<&str>) -> Result<Value, String> {
        let c = self.conn.lock().unwrap();
        if let Some(id) = id {
            let mut s = c.prepare("SELECT id,name,unique_id,connect_type,command,host,port,username,auth_type,encrypted_password,encrypted_private_key,term,cols,rows,prompt_regex,created_at,updated_at FROM profiles WHERE id=?")
                .map_err(|e| e.to_string())?;
            let v = s
                .query_row([id], row_profile)
                .optional()
                .map_err(|e| e.to_string())?
                .ok_or_else(|| "Profile not found".to_string())?;
            return Ok(v);
        }
        let mut s = c.prepare("SELECT id,name,unique_id,connect_type,command,host,port,username,auth_type,encrypted_password,encrypted_private_key,term,cols,rows,prompt_regex,created_at,updated_at FROM profiles ORDER BY created_at").map_err(|e| e.to_string())?;
        let rows = s.query_map([], row_profile).map_err(|e| e.to_string())?;
        let mut out = Vec::new();
        for row in rows {
            out.push(row.map_err(|e| e.to_string())?);
        }
        Ok(Value::Array(out))
    }
    pub fn sessions(&self) -> Result<Value, String> {
        let c = self.conn.lock().unwrap();
        let mut s = c.prepare("SELECT id,profile_id,profile_name,target,state,mode,cols,rows,created_at,closed_at,close_reason,exit_code,name,source FROM sessions ORDER BY created_at DESC, rowid DESC").map_err(|e| e.to_string())?;
        let rows = s.query_map([], |r| Ok(json!({"id":r.get::<_,String>(0)?,"profileId":r.get::<_,String>(1)?,"profileName":r.get::<_,String>(2)?,"target":r.get::<_,String>(3)?,"state":r.get::<_,String>(4)?,"mode":r.get::<_,String>(5)?,"cols":r.get::<_,i64>(6)?,"rows":r.get::<_,i64>(7)?,"createdAt":r.get::<_,i64>(8)?,"closedAt":r.get::<_,Option<i64>>(9)?,"closeReason":r.get::<_,Option<String>>(10)?,"exitCode":r.get::<_,Option<i64>>(11)?,"name":r.get::<_,String>(12)?,"source":r.get::<_,String>(13)?,"active":r.get::<_,String>(4)? != "DISCONNECTED"}))).map_err(|e| e.to_string())?;
        let mut out = Vec::new();
        for row in rows {
            out.push(row.map_err(|e| e.to_string())?);
        }
        Ok(Value::Array(out))
    }
    pub fn data_home(&self) -> &Path {
        &self.data_home
    }
    pub fn launch_profile(&self, id: &str) -> Result<LaunchProfile, String> {
        let c = self.conn.lock().unwrap();
        c.query_row(
            "SELECT name,connect_type,command,host,port,username,auth_type,encrypted_password,encrypted_private_key,encrypted_passphrase,term,cols,rows FROM profiles WHERE id=?",
            [id],
            |r| {
                Ok(LaunchProfile {
                    name: r.get(0)?,
                    connect_type: r.get(1)?,
                    command: r.get(2)?,
                    host: r.get(3)?,
                    port: r.get(4)?,
                    username: r.get(5)?,
                    auth_type: r.get(6)?,
                    password: r.get(7)?,
                    private_key: r.get(8)?,
                    passphrase: r.get(9)?,
                    term: r.get(10)?,
                    cols: r.get(11)?,
                    rows: r.get(12)?,
                })
            },
        )
        .optional()
        .map_err(|e| e.to_string())?
        .ok_or_else(|| "Profile not found".into())
    }
    pub fn decrypt_secret(&self, encoded: &str) -> Result<String, String> {
        decrypt(&self.key, encoded)
    }
    pub fn insert_session(
        &self,
        id: &str,
        profile_id: &str,
        profile_name: &str,
        target: &str,
        cols: i64,
        rows: i64,
        name: &str,
        source: &str,
        mode: &str,
    ) -> Result<(), String> {
        let c = self.conn.lock().unwrap();
        c.execute("INSERT INTO sessions (id,profile_id,profile_name,target,state,mode,cols,rows,created_at,name,source) VALUES (?,?,?,?,?,?,?,?,?,?,?)", rusqlite::params![id,profile_id,profile_name,target,"CONNECTING",mode,cols,rows,now(),name,source]).map_err(|e|e.to_string()).map(|_|())
    }
    pub fn update_session_state(
        &self,
        id: &str,
        state: &str,
        reason: Option<&str>,
    ) -> Result<(), String> {
        let c = self.conn.lock().unwrap();
        c.execute("UPDATE sessions SET state=?,closed_at=CASE WHEN ?='DISCONNECTED' THEN ? ELSE closed_at END,close_reason=CASE WHEN ?='DISCONNECTED' THEN ? ELSE close_reason END WHERE id=?", rusqlite::params![state,state,now(),state,reason,id]).map_err(|e|e.to_string()).map(|_|())
    }
    pub fn set_exit_code(&self, id: &str, code: Option<i32>) -> Result<(), String> {
        let c = self.conn.lock().unwrap();
        c.execute(
            "UPDATE sessions SET exit_code=? WHERE id=?",
            rusqlite::params![code.map(i64::from), id],
        )
        .map_err(|e| e.to_string())
        .map(|_| ())
    }
    /// 启动时调用: 此时本进程的 live 会话列表为空, 数据库里所有非 DISCONNECTED
    /// 的会话都是上次运行遗留的 (App 被强杀/崩溃时 shutdown 未执行)。
    pub fn reconcile_stale_sessions(&self) -> Result<(), String> {
        let c = self.conn.lock().unwrap();
        c.execute(
            "UPDATE sessions SET state='DISCONNECTED', closed_at=COALESCE(closed_at,?), close_reason='App exited without cleanup' WHERE state!='DISCONNECTED'",
            [now()],
        )
        .map_err(|e| e.to_string())
        .map(|_| ())
    }
    pub fn session_state(&self, id: &str) -> Result<Value, String> {
        let c = self.conn.lock().unwrap();
        c.query_row("SELECT id,state,mode,closed_at,close_reason,exit_code,(SELECT COALESCE(MAX(seq),0) FROM history_chunks WHERE session_id=sessions.id),(SELECT COALESCE(group_concat(data_plain,''),'') FROM (SELECT data_plain FROM (SELECT seq,data_plain FROM history_chunks WHERE session_id=sessions.id AND direction='output' ORDER BY seq DESC LIMIT 15) ORDER BY seq)) FROM sessions WHERE id=?", [id], |r| {
            let recent: String = r.get(7)?;
            let recent: String = recent.chars().rev().take(16_000).collect::<Vec<_>>().into_iter().rev().collect();
            Ok(json!({"id":r.get::<_,String>(0)?,"state":r.get::<_,String>(1)?,"mode":r.get::<_,String>(2)?,"closedAt":r.get::<_,Option<i64>>(3)?,"closeReason":r.get::<_,Option<String>>(4)?,"exitCode":r.get::<_,Option<i64>>(5)?,"active":r.get::<_,String>(1)? != "DISCONNECTED","lastSeq":r.get::<_,i64>(6)?,"recentOutput":recent}))
        }).optional().map_err(|e|e.to_string())?.ok_or_else(||"Session not found".into())
    }
    #[cfg(test)]
    pub fn add_output(&self, id: &str, text: &str) -> Result<i64, String> {
        self.add_output_plain(id, text, &crate::ansi::strip_ansi(text))
    }

    pub fn add_output_plain(&self, id: &str, text: &str, plain: &str) -> Result<i64, String> {
        let c = self.conn.lock().unwrap();
        let seq: i64 = c
            .query_row(
                "SELECT COALESCE(MAX(seq),0)+1 FROM history_chunks WHERE session_id=?",
                [id],
                |r| r.get(0),
            )
            .map_err(|e| e.to_string())?;
        // Pure control sequences (title changes, cursor moves, screen clears)
        // carry no readable text. Keep their raw bytes for replay/events but
        // hide them from history and read_output, which filter on internal=0.
        // Only chunks with no text at all are hidden; a chunk made of newlines
        // is real output and must stay visible to read_output.
        let internal = plain.is_empty();
        c.execute("INSERT INTO history_chunks (session_id,seq,direction,data_raw,data_plain,internal,created_at) VALUES (?,?,?,?,?,?,?)", rusqlite::params![id,seq,"output",text,plain,internal,now()]).map_err(|e|e.to_string())?;
        c.execute(
            "UPDATE sessions SET state='OUTPUTTING' WHERE id=? AND state!='DISCONNECTED'",
            [id],
        )
        .map_err(|e| e.to_string())?;
        Ok(seq)
    }
    /// MCP 开关与 token，存于 settings 表（历史上在 mcp.json，启动时迁移）。
    pub fn mcp_state(&self) -> Result<Value, String> {
        let raw: Option<String> = {
            let c = self.conn.lock().unwrap();
            c.query_row("SELECT value FROM settings WHERE key='mcp'", [], |r| {
                r.get(0)
            })
            .optional()
            .map_err(|e| e.to_string())?
        };
        let state = match raw {
            Some(raw) => serde_json::from_str::<Value>(&raw)
                .map_err(|error| format!("解析 MCP 配置失败: {error}"))?,
            None => json!({"enabled": true, "token": ""}),
        };
        validate_mcp_state(&state)?;
        Ok(state)
    }

    fn save_mcp_state(&self, state: &Value) -> Result<(), String> {
        validate_mcp_state(state)?;
        let raw = serde_json::to_string(state)
            .map_err(|error| format!("序列化 MCP 配置失败: {error}"))?;
        let c = self.conn.lock().unwrap();
        c.execute(
            "INSERT INTO settings (key,value) VALUES ('mcp',?1) ON CONFLICT(key) DO UPDATE SET value=?1",
            rusqlite::params![raw],
        )
        .map_err(|e| e.to_string())?;
        Ok(())
    }

    pub fn mcp_info(&self) -> Result<Value, String> {
        let mut state = self.mcp_state()?;
        if state
            .get("token")
            .and_then(Value::as_str)
            .unwrap_or("")
            .is_empty()
        {
            state["token"] = Value::String(random_token());
            self.save_mcp_state(&state)?;
        }
        let enabled = state
            .get("enabled")
            .and_then(Value::as_bool)
            .unwrap_or(true);
        let token = state.get("token").and_then(Value::as_str).unwrap_or("");
        let url = format!(
            "http://127.0.0.1:{}/shellink/mcp",
            std::env::var("SHELLINK_PORT")
                .ok()
                .and_then(|v| v.parse::<u16>().ok())
                .unwrap_or(7070)
        );
        let command = std::env::current_exe()
            .map(|path| path.to_string_lossy().into_owned())
            .unwrap_or_default();
        let mut stdio_env = json!({"SHELLINK_HOME":self.data_home.to_string_lossy()});
        if let Some(path) = self
            .conn
            .lock()
            .unwrap()
            .path()
            .filter(|path| !path.is_empty())
        {
            stdio_env["SHELLINK_DB"] = json!(path);
        }
        Ok(json!({
            "enabled":enabled,
            "running":false,
            "httpEnabled":enabled,
            "bindHost":std::env::var("SHELLINK_HOST").unwrap_or_else(|_| "127.0.0.1".into()),
            "url":url,
            "token":token,
            "command":command,
            "args":["mcp"],
            "stdioEnv":stdio_env,
            "clientConfig":{"mcpServers":{"shellink":{"url":url,"headers":{"Authorization":format!("Bearer {}",token)}}}},
            "clientConfigStdio":{"mcpServers":{"shellink":{"command":command,"args":["mcp"],"env":stdio_env}}}
        }))
    }
    pub fn mcp_config(&self, params: &Value) -> Result<Value, String> {
        let mut state = self.mcp_state()?;
        if let Some(value) = params.get("enabled") {
            let enabled = value
                .as_bool()
                .ok_or_else(|| "enabled must be a boolean".to_string())?;
            state["enabled"] = Value::Bool(enabled);
        }
        if let Some(value) = params.get("regenerateToken") {
            if value.as_bool().ok_or("regenerateToken must be a boolean")? {
                state["token"] = Value::String(random_token());
            }
        }
        self.save_mcp_state(&state)?;
        self.mcp_info()
    }
    /// Persist one submitted command. `raw` keeps the exact keystrokes while
    /// `display` is the readable form shown in history. Empty lines are never
    /// stored, so an input chunk always corresponds to an actual command.
    pub fn add_input_plain(&self, id: &str, raw: &str, display: &str) -> Result<(), String> {
        if display.trim().is_empty() {
            return Ok(());
        }
        let c = self.conn.lock().unwrap();
        let seq: i64 = c
            .query_row(
                "SELECT COALESCE(MAX(seq),0)+1 FROM history_chunks WHERE session_id=?",
                [id],
                |r| r.get(0),
            )
            .map_err(|e| e.to_string())?;
        c.execute("INSERT INTO history_chunks (session_id,seq,direction,data_raw,data_plain,internal,created_at) VALUES (?,?,?,?,?,?,?)", rusqlite::params![id,seq,"input",raw,display,0,now()]).map_err(|e|e.to_string()).map(|_|())
    }

    #[cfg(test)]
    pub fn add_input(&self, id: &str, text: &str) -> Result<(), String> {
        self.add_input_plain(id, text, text)
    }
    pub fn history_chunks(&self, id: &str, since: i64, limit: i64) -> Result<Value, String> {
        let c = self.conn.lock().unwrap();
        let exists: bool = c
            .query_row(
                "SELECT EXISTS(SELECT 1 FROM sessions WHERE id=?)",
                [id],
                |r| r.get(0),
            )
            .map_err(|e| e.to_string())?;
        if !exists {
            return Err("Session not found".into());
        }
        let limit = limit.clamp(1, 10_000);
        let mut s=c.prepare("SELECT id,session_id,seq,direction,data_plain,created_at FROM history_chunks WHERE session_id=? AND seq>? AND internal=0 ORDER BY seq LIMIT ?").map_err(|e|e.to_string())?;
        let rows=s.query_map(rusqlite::params![id,since,limit],|r|Ok(json!({"id":r.get::<_,i64>(0)?,"sessionId":r.get::<_,String>(1)?,"seq":r.get::<_,i64>(2)?,"direction":r.get::<_,String>(3)?,"plain":r.get::<_,String>(4)?,"createdAt":r.get::<_,i64>(5)?}))).map_err(|e|e.to_string())?;
        let mut items = Vec::new();
        let mut cursor = since;
        for row in rows {
            let v = row.map_err(|e| e.to_string())?;
            cursor = v.get("seq").and_then(Value::as_i64).unwrap_or(cursor);
            items.push(v);
        }
        Ok(json!({"items":items,"cursor":cursor,"hasMore":items.len() as i64==limit}))
    }
    /// Return the latest lines, including an unfinished final line such as a prompt.
    pub fn read_output(&self, id: &str, limit: usize) -> Result<Value, String> {
        if !(1..=10_000).contains(&limit) {
            return Err("invalid output line limit".into());
        }
        let c = self.conn.lock().unwrap();
        let exists: bool = c
            .query_row(
                "SELECT EXISTS(SELECT 1 FROM sessions WHERE id=?)",
                [id],
                |row| row.get(0),
            )
            .map_err(|e| e.to_string())?;
        if !exists {
            return Err("Session not found".into());
        }
        let mut statement = c.prepare(
            "SELECT data_plain FROM history_chunks WHERE session_id=? AND direction='output' AND internal=0 ORDER BY seq DESC",
        ).map_err(|e| e.to_string())?;
        let mut rows = statement.query([id]).map_err(|e| e.to_string())?;
        let mut reversed = Vec::new();
        let mut remaining = limit;
        let mut has_earlier = false;
        while let Some(row) = rows.next().map_err(|e| e.to_string())? {
            let chunk: String = row.get(0).map_err(|e| e.to_string())?;
            for ch in chunk.chars().rev() {
                // A trailing newline terminates the final line without adding an empty one.
                if ch == '\n' && !reversed.is_empty() {
                    remaining -= 1;
                    if remaining == 0 {
                        has_earlier = true;
                        break;
                    }
                }
                reversed.push(ch);
            }
            if has_earlier {
                break;
            }
        }
        let text: String = reversed.into_iter().rev().collect();
        Ok(json!({"text":text,"hasEarlier":has_earlier}))
    }
    pub fn update_session_mode(&self, id: &str, mode: &str) -> Result<Value, String> {
        if mode != "AUTO" && mode != "MANUAL" {
            return Err("mode must be AUTO or MANUAL".into());
        }
        let c = self.conn.lock().unwrap();
        if c.execute(
            "UPDATE sessions SET mode=? WHERE id=?",
            rusqlite::params![mode, id],
        )
        .map_err(|e| e.to_string())?
            == 0
        {
            return Err("Session not found".into());
        }
        Ok(json!({"ok":true,"mode":mode}))
    }
    pub fn session_mode(&self, id: &str) -> Result<String, String> {
        self.conn
            .lock()
            .unwrap()
            .query_row("SELECT mode FROM sessions WHERE id=?", [id], |r| r.get(0))
            .optional()
            .map_err(|e| e.to_string())?
            .ok_or_else(|| "Session not found".into())
    }
    pub fn update_session_size(&self, id: &str, cols: i64, rows: i64) -> Result<Value, String> {
        let c = self.conn.lock().unwrap();
        if c.execute(
            "UPDATE sessions SET cols=?,rows=? WHERE id=?",
            rusqlite::params![cols, rows, id],
        )
        .map_err(|e| e.to_string())?
            == 0
        {
            return Err("Session not found".into());
        }
        Ok(json!({"ok":true}))
    }
    pub fn rename_session(&self, id: &str, name: &str) -> Result<Value, String> {
        if name.trim().is_empty() {
            return Err("name must be between 1 and 80 characters".into());
        }
        if name.chars().count() > 80 {
            return Err("name must be at most 80 characters".into());
        }
        let c = self.conn.lock().unwrap();
        if c.execute(
            "UPDATE sessions SET name=? WHERE id=?",
            rusqlite::params![name.trim(), id],
        )
        .map_err(|e| e.to_string())?
            == 0
        {
            return Err("Session not found".into());
        }
        Ok(json!({"ok":true}))
    }
    pub fn remove_session(&self, id: &str) -> Result<Value, String> {
        let c = self.conn.lock().unwrap();
        c.execute("DELETE FROM history_chunks WHERE session_id=?", [id])
            .map_err(|e| e.to_string())?;
        if c.execute("DELETE FROM sessions WHERE id=?", [id])
            .map_err(|e| e.to_string())?
            == 0
        {
            return Err("Session not found".into());
        }
        Ok(json!({"ok":true}))
    }
    pub fn replay(&self, id: &str) -> Result<Value, String> {
        let c = self.conn.lock().unwrap();
        let exists: bool = c
            .query_row(
                "SELECT EXISTS(SELECT 1 FROM sessions WHERE id=?)",
                [id],
                |r| r.get(0),
            )
            .map_err(|e| e.to_string())?;
        if !exists {
            return Err("Session not found".into());
        }
        let mut statement = c.prepare("SELECT data_raw,seq FROM history_chunks WHERE session_id=? AND direction='output' ORDER BY seq").map_err(|e| e.to_string())?;
        let rows = statement
            .query_map([id], |row| {
                Ok((row.get::<_, String>(0)?, row.get::<_, i64>(1)?))
            })
            .map_err(|e| e.to_string())?;
        let mut raw = String::new();
        let mut cursor = 0;
        for row in rows {
            let (text, seq) = row.map_err(|e| e.to_string())?;
            raw.push_str(&text);
            cursor = seq;
        }
        Ok(json!({"raw":raw,"cursor":cursor}))
    }
    pub fn profile_create(&self, p: &Value) -> Result<Value, String> {
        let name = p
            .get("name")
            .and_then(Value::as_str)
            .filter(|s| !s.is_empty())
            .ok_or("name must be a non-empty string")?;
        let id = format!("rust-{}", now());
        let t = now();
        let password = self.encrypt_field(p, "password")?;
        let private_key = self.encrypt_field(p, "privateKey")?;
        let passphrase = self.encrypt_field(p, "passphrase")?;
        let conn = self.conn.lock().unwrap();
        conn.execute("INSERT INTO profiles (id,name,unique_id,connect_type,command,host,port,username,auth_type,encrypted_password,encrypted_private_key,encrypted_passphrase,term,cols,rows,prompt_regex,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)",
            rusqlite::params![id,name,p.get("uniqueId").and_then(Value::as_str),p.get("connectType").and_then(Value::as_str).unwrap_or("ssh"),p.get("command").and_then(Value::as_str),p.get("host").and_then(Value::as_str).unwrap_or(""),p.get("port").and_then(Value::as_i64).unwrap_or(22),p.get("username").and_then(Value::as_str).unwrap_or(""),p.get("authType").and_then(Value::as_str).unwrap_or("password"),password,private_key,passphrase,p.get("term").and_then(Value::as_str).unwrap_or("xterm-256color"),p.get("cols").and_then(Value::as_i64).unwrap_or(160),p.get("rows").and_then(Value::as_i64).unwrap_or(42),p.get("promptRegex").and_then(Value::as_str),t,t]).map_err(|e| e.to_string())?;
        drop(conn);
        self.profiles(Some(&id))
    }
    pub fn profile_update(&self, id: &str, p: &Value) -> Result<Value, String> {
        let conn = self.conn.lock().unwrap();
        let changed = conn.execute("UPDATE profiles SET name=COALESCE(?,name),unique_id=COALESCE(?,unique_id),connect_type=COALESCE(?,connect_type),command=COALESCE(?,command),host=COALESCE(?,host),port=COALESCE(?,port),username=COALESCE(?,username),auth_type=COALESCE(?,auth_type),encrypted_password=CASE WHEN ? THEN ? ELSE encrypted_password END,encrypted_private_key=CASE WHEN ? THEN ? ELSE encrypted_private_key END,encrypted_passphrase=CASE WHEN ? THEN ? ELSE encrypted_passphrase END,term=COALESCE(?,term),cols=COALESCE(?,cols),rows=COALESCE(?,rows),prompt_regex=COALESCE(?,prompt_regex),updated_at=? WHERE id=?", rusqlite::params![p.get("name").and_then(Value::as_str),p.get("uniqueId").and_then(Value::as_str),p.get("connectType").and_then(Value::as_str),p.get("command").and_then(Value::as_str),p.get("host").and_then(Value::as_str),p.get("port").and_then(Value::as_i64),p.get("username").and_then(Value::as_str),p.get("authType").and_then(Value::as_str),p.get("password").is_some(),self.encrypt_field(p,"password")?,p.get("privateKey").is_some(),self.encrypt_field(p,"privateKey")?,p.get("passphrase").is_some(),self.encrypt_field(p,"passphrase")?,p.get("term").and_then(Value::as_str),p.get("cols").and_then(Value::as_i64),p.get("rows").and_then(Value::as_i64),p.get("promptRegex").and_then(Value::as_str),now(),id]).map_err(|e| e.to_string())?;
        if changed == 0 {
            return Err("Profile not found".into());
        }
        drop(conn);
        self.profiles(Some(id))
    }
    fn encrypt_field(&self, p: &Value, name: &str) -> Result<Option<String>, String> {
        match p.get(name) {
            None | Some(Value::Null) => Ok(None),
            Some(v) => {
                let s = v
                    .as_str()
                    .ok_or_else(|| format!("{name} must be a string"))?;
                if s.is_empty() {
                    return Ok(None);
                }
                encrypt(&self.key, s).map(Some)
            }
        }
    }
    pub fn profile_delete(&self, id: &str) -> Result<Value, String> {
        let c = self.conn.lock().unwrap();
        if c.execute("DELETE FROM profiles WHERE id=?", [id])
            .map_err(|e| e.to_string())?
            == 0
        {
            return Err("Profile not found".into());
        }
        Ok(json!({"ok":true}))
    }
}

pub(crate) fn session_now() -> i64 {
    now()
}
fn now() -> i64 {
    std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap_or_default()
        .as_millis() as i64
}

fn load_master_key(home: &Path, env_value: Option<String>) -> Result<[u8; 32], String> {
    let file = home.join("master.key");
    let env_key = env_value
        .as_deref()
        .filter(|raw| !raw.is_empty())
        .map(decode_key)
        .transpose()?;
    match fs::read_to_string(&file) {
        Ok(raw) => {
            let key = decode_key(raw.trim())?;
            if env_key.is_some_and(|env| env != key) {
                return Err("SHELLINK_MASTER_KEY does not match existing master.key".into());
            }
            check_key_permissions(&file)?;
            Ok(key)
        }
        Err(e) if e.kind() == std::io::ErrorKind::NotFound => {
            let key = if let Some(key) = env_key {
                key
            } else {
                let mut key = [0u8; 32];
                OsRng.fill_bytes(&mut key);
                key
            };
            let encoded = key.iter().map(|b| format!("{:02x}", b)).collect::<String>() + "\n";
            write_master_key(&file, encoded.as_bytes())?;
            Ok(key)
        }
        Err(e) => Err(format!("读取 master.key 失败: {e}")),
    }
}
#[cfg(unix)]
fn check_key_permissions(path: &Path) -> Result<(), String> {
    use std::os::unix::fs::PermissionsExt;
    let mode = fs::metadata(path)
        .map_err(|e| e.to_string())?
        .permissions()
        .mode()
        & 0o777;
    if mode != 0o600 {
        return Err(format!("{} permissions must be 0600", path.display()));
    }
    Ok(())
}
#[cfg(not(unix))]
fn check_key_permissions(_path: &Path) -> Result<(), String> {
    Ok(())
}
#[cfg(unix)]
fn write_master_key(path: &Path, bytes: &[u8]) -> Result<(), String> {
    use std::os::unix::fs::OpenOptionsExt;
    let mut file = fs::OpenOptions::new()
        .write(true)
        .create_new(true)
        .mode(0o600)
        .open(path)
        .map_err(|e| format!("创建 master.key 失败: {e}"))?;
    use std::io::Write;
    file.write_all(bytes)
        .map_err(|e| format!("写入 master.key 失败: {e}"))?;
    file.sync_all()
        .map_err(|e| format!("同步 master.key 失败: {e}"))?;
    check_key_permissions(path)
}
#[cfg(not(unix))]
fn write_master_key(path: &Path, bytes: &[u8]) -> Result<(), String> {
    let mut file = fs::OpenOptions::new()
        .write(true)
        .create_new(true)
        .open(path)
        .map_err(|e| format!("创建 master.key 失败: {e}"))?;
    use std::io::Write;
    file.write_all(bytes)
        .map_err(|e| format!("写入 master.key 失败: {e}"))?;
    file.sync_all()
        .map_err(|e| format!("同步 master.key 失败: {e}"))?;
    Ok(())
}
fn resolve_data_home() -> Result<PathBuf, String> {
    let explicit = std::env::var_os("SHELLINK_HOME")
        .filter(|value| !value.is_empty())
        .map(PathBuf::from);
    if let Some(path) = explicit {
        return make_absolute(path);
    }
    let appdata = std::env::var_os("APPDATA").map(PathBuf::from);
    let home = std::env::var_os("HOME").map(PathBuf::from);
    let user_profile = std::env::var_os("USERPROFILE").map(PathBuf::from);
    let portable = portable_data_home();
    if portable.as_deref().is_some_and(portable_requested) {
        return make_absolute(portable.unwrap());
    }
    let default = resolve_data_home_from(None, appdata, home, user_profile, cfg!(windows))?;
    let default_error = ensure_private_dir(&default).err();
    if default_error.is_none() && writable_directory(&default) {
        return Ok(default);
    }
    if has_state(&default) {
        return Ok(default);
    }
    if let Some(portable) = portable {
        if ensure_private_dir(&portable).is_ok() && writable_directory(&portable) {
            return make_absolute(portable);
        }
    }
    Err(format!(
        "无法写入数据目录 {}，请设置 SHELLINK_HOME 到可写目录{}",
        default.display(),
        default_error
            .map(|error| format!(": {error}"))
            .unwrap_or_default()
    ))
}

fn portable_data_home() -> Option<PathBuf> {
    let executable = std::env::current_exe().ok()?;
    Some(executable.parent()?.join("shellink-data"))
}

fn portable_requested(data_home: &Path) -> bool {
    data_home
        .parent()
        .map(|parent| parent.join("shellink.portable").is_file())
        .unwrap_or(false)
}

fn writable_directory(path: &Path) -> bool {
    let probe = path.join(format!(
        ".shellink-probe-{}-{}",
        std::process::id(),
        random_token()
    ));
    match fs::OpenOptions::new()
        .write(true)
        .create_new(true)
        .open(&probe)
    {
        Ok(file) => {
            drop(file);
            fs::remove_file(probe).is_ok()
        }
        Err(_) => false,
    }
}

fn has_state(path: &Path) -> bool {
    ["shellink.db", "master.key"]
        .iter()
        .any(|name| path.join(name).is_file())
}
fn resolve_data_home_from(
    explicit: Option<PathBuf>,
    appdata: Option<PathBuf>,
    home: Option<PathBuf>,
    user_profile: Option<PathBuf>,
    windows: bool,
) -> Result<PathBuf, String> {
    let path = if let Some(path) = explicit {
        path
    } else if windows {
        appdata
            .or_else(|| {
                user_profile
                    .or(home)
                    .map(|path| path.join("AppData").join("Roaming"))
            })
            .ok_or_else(|| "无法确定 Shellink 数据目录".to_string())?
            .join("Shellink")
    } else {
        home.ok_or_else(|| "无法确定 Shellink 数据目录".to_string())?
            .join(".Shellink")
    };
    make_absolute(path)
}
fn make_absolute(path: PathBuf) -> Result<PathBuf, String> {
    if path.is_absolute() {
        Ok(path)
    } else {
        std::env::current_dir()
            .map(|cwd| cwd.join(path))
            .map_err(|e| e.to_string())
    }
}
#[cfg(unix)]
fn ensure_private_dir(path: &Path) -> Result<(), String> {
    use std::os::unix::fs::DirBuilderExt;
    let mut builder = fs::DirBuilder::new();
    builder.recursive(true).mode(0o700);
    builder
        .create(path)
        .map_err(|e| format!("创建数据目录失败: {e}"))
}
#[cfg(not(unix))]
fn ensure_private_dir(path: &Path) -> Result<(), String> {
    fs::create_dir_all(path).map_err(|e| format!("创建数据目录失败: {e}"))
}
fn initialize_schema(conn: &Connection) -> Result<(), String> {
    conn.execute_batch("CREATE TABLE IF NOT EXISTS profiles (id TEXT PRIMARY KEY,name TEXT NOT NULL,unique_id TEXT,connect_type TEXT NOT NULL DEFAULT 'ssh',command TEXT,host TEXT NOT NULL DEFAULT '',port INTEGER NOT NULL DEFAULT 22,username TEXT NOT NULL DEFAULT '',auth_type TEXT NOT NULL DEFAULT 'password',encrypted_password TEXT,encrypted_private_key TEXT,encrypted_passphrase TEXT,term TEXT NOT NULL DEFAULT 'xterm-256color',cols INTEGER NOT NULL DEFAULT 160,rows INTEGER NOT NULL DEFAULT 42,prompt_regex TEXT,created_at INTEGER NOT NULL,updated_at INTEGER NOT NULL); CREATE TABLE IF NOT EXISTS sessions (id TEXT PRIMARY KEY,profile_id TEXT NOT NULL,profile_name TEXT NOT NULL,target TEXT NOT NULL,state TEXT NOT NULL,mode TEXT NOT NULL DEFAULT 'AUTO',cols INTEGER NOT NULL,rows INTEGER NOT NULL,created_at INTEGER NOT NULL,closed_at INTEGER,close_reason TEXT,exit_code INTEGER,name TEXT NOT NULL DEFAULT '',source TEXT NOT NULL DEFAULT 'unknown'); CREATE TABLE IF NOT EXISTS history_chunks (id INTEGER PRIMARY KEY AUTOINCREMENT,session_id TEXT NOT NULL,seq INTEGER NOT NULL,direction TEXT NOT NULL,data_raw TEXT NOT NULL,data_plain TEXT NOT NULL,internal INTEGER NOT NULL DEFAULT 0,created_at INTEGER NOT NULL); CREATE TABLE IF NOT EXISTS settings (key TEXT PRIMARY KEY,value TEXT NOT NULL); CREATE UNIQUE INDEX IF NOT EXISTS idx_profiles_unique_id ON profiles(unique_id) WHERE unique_id IS NOT NULL; CREATE INDEX IF NOT EXISTS idx_history_session_seq ON history_chunks(session_id,seq);").map_err(|e|e.to_string())
}
fn decode_key(raw: &str) -> Result<[u8; 32], String> {
    if raw.len() != 64 {
        return Err("master key must be 32 bytes".into());
    }
    let mut out = [0u8; 32];
    for (i, pair) in raw.as_bytes().chunks_exact(2).enumerate() {
        out[i] = hex_nibble(pair[0])
            .and_then(|a| hex_nibble(pair[1]).map(|b| a * 16 + b))
            .ok_or("SHELLINK_MASTER_KEY must be hex")?;
    }
    Ok(out)
}
fn hex_nibble(c: u8) -> Option<u8> {
    match c {
        b'0'..=b'9' => Some(c - b'0'),
        b'a'..=b'f' => Some(c - b'a' + 10),
        b'A'..=b'F' => Some(c - b'A' + 10),
        _ => None,
    }
}
fn random_token() -> String {
    let mut bytes = [0u8; 24];
    OsRng.fill_bytes(&mut bytes);
    bytes.iter().map(|b| format!("{:02x}", b)).collect()
}

fn validate_mcp_state(state: &Value) -> Result<(), String> {
    let object = state
        .as_object()
        .ok_or_else(|| "MCP 配置必须是 JSON 对象".to_string())?;
    if let Some(enabled) = object.get("enabled") {
        if !enabled.is_boolean() {
            return Err("MCP 配置 enabled 必须是布尔值".into());
        }
    }
    if let Some(token) = object.get("token") {
        if !token.is_string() {
            return Err("MCP 配置 token 必须是字符串".into());
        }
    }
    Ok(())
}

/// 旧版本把 MCP 开关/token 存在数据目录的 mcp.json，迁移进 settings 表后删除该文件。
/// 解析失败时保留文件并报错，避免丢配置；settings 表已有值时不覆盖。
fn migrate_legacy_mcp_json(conn: &Connection, data_home: &Path) -> Result<(), String> {
    let path = data_home.join("mcp.json");
    let raw = match fs::read_to_string(&path) {
        Ok(raw) => raw,
        Err(error) if error.kind() == std::io::ErrorKind::NotFound => return Ok(()),
        Err(error) => return Err(format!("读取 MCP 配置失败: {error}")),
    };
    let state: Value =
        serde_json::from_str(&raw).map_err(|error| format!("解析 MCP 配置失败: {error}"))?;
    validate_mcp_state(&state)?;
    let exists: bool = conn
        .query_row(
            "SELECT EXISTS(SELECT 1 FROM settings WHERE key='mcp')",
            [],
            |r| r.get(0),
        )
        .map_err(|e| e.to_string())?;
    if !exists {
        conn.execute(
            "INSERT INTO settings (key,value) VALUES ('mcp',?1)",
            rusqlite::params![serde_json::to_string(&state).map_err(|e| e.to_string())?],
        )
        .map_err(|e| e.to_string())?;
    }
    fs::remove_file(&path).map_err(|error| format!("删除旧 MCP 配置失败: {error}"))?;
    Ok(())
}

fn encrypt(key: &[u8; 32], plain: &str) -> Result<String, String> {
    let cipher = Aes256Gcm::new_from_slice(key).map_err(|e| e.to_string())?;
    let mut nonce = [0u8; 12];
    OsRng.fill_bytes(&mut nonce);
    let sealed = cipher
        .encrypt(Nonce::from_slice(&nonce), plain.as_bytes())
        .map_err(|e| e.to_string())?;
    let tag_at = sealed.len() - 16;
    let mut out = Vec::with_capacity(sealed.len() + 12);
    out.extend_from_slice(&nonce);
    out.extend_from_slice(&sealed[tag_at..]);
    out.extend_from_slice(&sealed[..tag_at]);
    Ok(STANDARD.encode(out))
}

fn decrypt(key: &[u8; 32], encoded: &str) -> Result<String, String> {
    let raw = STANDARD
        .decode(encoded)
        .map_err(|e| format!("解密字段编码无效: {e}"))?;
    if raw.len() < 28 {
        return Err("解密字段长度无效".into());
    }
    let cipher = Aes256Gcm::new_from_slice(key).map_err(|e| e.to_string())?;
    let nonce = Nonce::from_slice(&raw[..12]);
    let mut sealed = raw[28..].to_vec();
    sealed.extend_from_slice(&raw[12..28]);
    let plain = cipher
        .decrypt(nonce, sealed.as_ref())
        .map_err(|_| "解密字段失败".to_string())?;
    String::from_utf8(plain).map_err(|e| format!("解密字段不是 UTF-8: {e}"))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn recent_output_is_chronological_and_bounded() {
        let db = DataStore::test_store(PathBuf::new());
        db.insert_session("s", "p", "Profile", "local", 120, 36, "", "test", "AUTO")
            .unwrap();
        for chunk in ["first\n", "second\n", "third\n"] {
            db.add_output("s", chunk).unwrap();
        }
        assert_eq!(
            db.session_state("s").unwrap()["recentOutput"],
            "first\nsecond\nthird\n"
        );
        db.add_output("s", &"中".repeat(20_000)).unwrap();
        assert_eq!(
            db.session_state("s").unwrap()["recentOutput"]
                .as_str()
                .unwrap()
                .chars()
                .count(),
            16_000
        );
    }

    #[test]
    fn output_snapshots_keep_complete_recent_lines() {
        let db = DataStore::test_store(PathBuf::new());
        db.insert_session("s", "p", "Profile", "local", 120, 36, "", "test", "AUTO")
            .unwrap();
        db.add_output("s", "first\n").unwrap();
        db.add_input("s", "secret input").unwrap();
        db.add_output("s", "second\n第三行\n").unwrap();
        assert_eq!(
            db.read_output("s", 1).unwrap(),
            json!({"text":"第三行\n","hasEarlier":true})
        );
        assert_eq!(
            db.read_output("s", 2).unwrap(),
            json!({"text":"second\n第三行\n","hasEarlier":true})
        );
        assert_eq!(
            db.read_output("s", 3).unwrap(),
            json!({"text":"first\nsecond\n第三行\n","hasEarlier":false})
        );
        let long_line = format!("{}最新结果\n", "中".repeat(70_000));
        db.add_output("s", &long_line).unwrap();
        assert_eq!(
            db.read_output("s", 1).unwrap(),
            json!({"text":long_line,"hasEarlier":true})
        );
    }

    #[test]
    fn output_snapshots_join_lines_across_chunks_and_keep_the_final_prompt() {
        let db = DataStore::test_store(PathBuf::new());
        db.insert_session("s", "p", "Profile", "local", 120, 36, "", "test", "AUTO")
            .unwrap();
        for chunk in [
            "first\r",
            "\nsec",
            "ond\r\n",
            "第三",
            "行\r",
            "\nprompt> ",
            "",
        ] {
            db.add_output("s", chunk).unwrap();
        }
        assert_eq!(
            db.read_output("s", 1).unwrap(),
            json!({"text":"prompt> ","hasEarlier":true})
        );
        assert_eq!(
            db.read_output("s", 2).unwrap(),
            json!({"text":"第三行\r\nprompt> ","hasEarlier":true})
        );
        assert_eq!(
            db.read_output("s", 4).unwrap(),
            json!({"text":"first\r\nsecond\r\n第三行\r\nprompt> ","hasEarlier":false})
        );
    }

    #[test]
    fn output_snapshots_handle_blank_lines_and_do_not_limit_chunk_count() {
        let db = DataStore::test_store(PathBuf::new());
        db.insert_session("s", "p", "Profile", "local", 120, 36, "", "test", "AUTO")
            .unwrap();
        db.add_output("s", "\n\n").unwrap();
        assert_eq!(
            db.read_output("s", 1).unwrap(),
            json!({"text":"\n","hasEarlier":true})
        );
        assert_eq!(
            db.read_output("s", 2).unwrap(),
            json!({"text":"\n\n","hasEarlier":false})
        );
        for _ in 0..500 {
            db.add_output("s", "中").unwrap();
        }
        assert_eq!(
            db.read_output("s", 1).unwrap(),
            json!({"text":"中".repeat(500),"hasEarlier":true})
        );
    }

    #[test]
    fn output_snapshots_are_independent_and_exclude_other_sessions_and_internal_logs() {
        let db = DataStore::test_store(PathBuf::new());
        for id in ["s", "other", "empty"] {
            db.insert_session(id, "p", "Profile", "local", 120, 36, "", "test", "AUTO")
                .unwrap();
        }
        db.add_output("s", "visible").unwrap();
        db.add_output("other", "other output").unwrap();
        let internal_seq = db.add_output("s", "internal output").unwrap();
        db.conn
            .lock()
            .unwrap()
            .execute(
                "UPDATE history_chunks SET internal=1 WHERE session_id=? AND seq=?",
                rusqlite::params!["s", internal_seq],
            )
            .unwrap();
        let first_reader = db.read_output("s", 100).unwrap();
        let second_reader = db.read_output("s", 100).unwrap();
        assert_eq!(first_reader, json!({"text":"visible","hasEarlier":false}));
        assert_eq!(second_reader, first_reader);
        assert_eq!(
            db.read_output("empty", 100).unwrap(),
            json!({"text":"","hasEarlier":false})
        );
        assert!(db.read_output("missing", 100).is_err());
        for limit in [0, 10_001] {
            assert!(db.read_output("s", limit).is_err());
        }
    }

    #[test]
    fn control_only_output_stays_raw_but_is_hidden_from_history() {
        let db = DataStore::test_store(PathBuf::new());
        db.insert_session("s", "p", "Profile", "local", 120, 36, "", "test", "AUTO")
            .unwrap();
        db.add_output_plain("s", "\x1b]0;title\x07\x1b[H\x1b[J", "")
            .unwrap();
        db.add_output("s", "visible\n").unwrap();

        let chunks = db.history_chunks("s", 0, 100).unwrap();
        let items = chunks["items"].as_array().unwrap();
        assert_eq!(items.len(), 1);
        assert_eq!(items[0]["plain"], "visible\n");
        assert_eq!(db.read_output("s", 100).unwrap()["text"], "visible\n");
        // Raw bytes remain available for terminal replay.
        assert!(
            db.replay("s").unwrap()["raw"]
                .as_str()
                .unwrap()
                .contains("title")
        );
    }

    #[test]
    fn empty_input_lines_are_never_stored() {
        let db = DataStore::test_store(PathBuf::new());
        db.insert_session("s", "p", "Profile", "local", 120, 36, "", "test", "AUTO")
            .unwrap();
        db.add_input_plain("s", "\x1b[A", "").unwrap();
        db.add_input_plain("s", "ls\r", "ls").unwrap();
        let chunks = db.history_chunks("s", 0, 100).unwrap();
        let items = chunks["items"].as_array().unwrap();
        assert_eq!(items.len(), 1);
        assert_eq!(items[0]["plain"], "ls");
    }

    fn test_dir(label: &str) -> PathBuf {
        let path = std::env::temp_dir().join(format!(
            "shellink-{label}-{}-{}",
            std::process::id(),
            random_token()
        ));
        fs::create_dir(&path).unwrap();
        path
    }

    #[test]
    fn replay_cursor_tracks_output_sequence_with_interleaved_input() {
        let conn = Connection::open_in_memory().unwrap();
        initialize_schema(&conn).unwrap();
        let db = DataStore {
            conn: Mutex::new(conn),
            key: [0; 32],
            data_home: PathBuf::new(),
        };
        db.insert_session(
            "test", "profile", "Profile", "local", 120, 36, "", "test", "MANUAL",
        )
        .unwrap();
        assert_eq!(db.add_output("test", "prompt> ").unwrap(), 1);
        db.add_input("test", "pwd\n").unwrap();
        assert_eq!(db.add_output("test", "/tmp\n").unwrap(), 3);
        assert_eq!(
            db.replay("test").unwrap(),
            json!({"raw":"prompt> /tmp\n", "cursor":3})
        );
        db.update_session_state("test", "DISCONNECTED", Some("Process exited"))
            .unwrap();
        db.add_output("test", "final output").unwrap();
        assert_eq!(db.session_state("test").unwrap()["active"], false);
    }

    #[test]
    fn reconcile_marks_orphaned_active_sessions_as_disconnected() {
        let conn = Connection::open_in_memory().unwrap();
        initialize_schema(&conn).unwrap();
        let db = DataStore {
            conn: Mutex::new(conn),
            key: [0; 32],
            data_home: PathBuf::new(),
        };
        // 上次运行遗留的"活跃"会话 (进程被强杀时 shutdown 未执行)
        db.insert_session(
            "stale-1", "profile", "Profile", "local", 120, 36, "", "test", "MANUAL",
        )
        .unwrap();
        db.insert_session(
            "stale-2", "profile", "Profile", "local", 120, 36, "", "test", "AUTO",
        )
        .unwrap();
        db.update_session_state("stale-2", "OUTPUTTING", None)
            .unwrap();
        db.reconcile_stale_sessions().unwrap();
        assert_eq!(db.session_state("stale-1").unwrap()["active"], false);
        assert_eq!(db.session_state("stale-2").unwrap()["active"], false);
        assert_eq!(
            db.session_state("stale-1").unwrap()["closeReason"],
            "App exited without cleanup"
        );
        // 本次运行的真实会话 (reconcile 之后创建), 不应被影响
        db.insert_session(
            "live-1", "profile", "Profile", "local", 120, 36, "", "test", "MANUAL",
        )
        .unwrap();
        assert_eq!(db.session_state("live-1").unwrap()["active"], true);
        // 幂等: 重复调用不覆盖已有 closed_at / close_reason
        db.reconcile_stale_sessions().unwrap();
        let again = db.session_state("stale-1").unwrap();
        assert_eq!(again["active"], false);
        assert_eq!(again["closeReason"], "App exited without cleanup");
    }

    #[test]
    fn sessions_list_is_newest_first() {
        let db = DataStore::test_store(PathBuf::new());
        db.insert_session("old", "p", "Profile", "local", 120, 36, "", "test", "AUTO")
            .unwrap();
        std::thread::sleep(std::time::Duration::from_millis(5));
        db.insert_session("new", "p", "Profile", "local", 120, 36, "", "test", "AUTO")
            .unwrap();
        let list = db.sessions().unwrap();
        let ids: Vec<&str> = list
            .as_array()
            .unwrap()
            .iter()
            .map(|s| s["id"].as_str().unwrap())
            .collect();
        assert_eq!(ids, vec!["new", "old"]);
    }

    #[test]
    fn resolves_data_home_using_platform_defaults() {
        let home = PathBuf::from("/users/shellink");
        assert_eq!(
            resolve_data_home_from(None, None, Some(home.clone()), None, false).unwrap(),
            home.join(".Shellink")
        );
        assert_eq!(
            resolve_data_home_from(
                None,
                Some(PathBuf::from("/appdata")),
                Some(home.clone()),
                None,
                true
            )
            .unwrap(),
            PathBuf::from("/appdata/Shellink")
        );
        assert_eq!(
            resolve_data_home_from(Some(PathBuf::from("/custom")), None, Some(home), None, true,)
                .unwrap(),
            PathBuf::from("/custom")
        );
        assert_eq!(
            resolve_data_home_from(
                None,
                None,
                None,
                Some(PathBuf::from("/users/shellink")),
                true,
            )
            .unwrap(),
            PathBuf::from("/users/shellink/AppData/Roaming/Shellink")
        );
    }

    #[test]
    fn master_key_is_persisted_in_data_home_and_validated() {
        let home = test_dir("key");
        let key = load_master_key(&home, None).unwrap();
        assert_eq!(load_master_key(&home, None).unwrap(), key);
        assert!(home.join("master.key").is_file());
        let other = [7_u8; 32];
        let other_hex = other
            .iter()
            .map(|byte| format!("{byte:02x}"))
            .collect::<String>();
        assert!(load_master_key(&home, Some(other_hex)).is_err());
        #[cfg(unix)]
        {
            use std::os::unix::fs::PermissionsExt;
            assert_eq!(
                fs::metadata(home.join("master.key"))
                    .unwrap()
                    .permissions()
                    .mode()
                    & 0o777,
                0o600
            );
        }
        fs::remove_dir_all(home).unwrap();
    }

    #[test]
    fn mcp_state_persists_in_settings_table_and_migrates_mcp_json() {
        let conn = Connection::open_in_memory().unwrap();
        initialize_schema(&conn).unwrap();
        let db = DataStore {
            conn: Mutex::new(conn),
            key: [0; 32],
            data_home: PathBuf::new(),
        };
        // 默认值: enabled=true, token 为空
        assert_eq!(
            db.mcp_state().unwrap(),
            json!({"enabled": true, "token": ""})
        );
        // 配置写入 settings 表
        let info = db
            .mcp_config(&json!({"enabled": false, "regenerateToken": true}))
            .unwrap();
        assert_eq!(info["enabled"], json!(false));
        assert_ne!(info["token"], json!(""));
        assert_eq!(
            db.mcp_state().unwrap(),
            json!({"enabled": false, "token": info["token"]})
        );
        // 空token首次读取时自动生成并持久化
        let first = db.mcp_info().unwrap();
        assert_ne!(first["token"], json!(""));
        assert_eq!(
            db.mcp_state().unwrap(),
            json!({"enabled": false, "token": first["token"]})
        );
    }

    #[test]
    fn migration_imports_legacy_mcp_json_into_settings_and_removes_it() {
        let home = test_dir("mcp-migrate");
        let conn = Connection::open_in_memory().unwrap();
        initialize_schema(&conn).unwrap();
        let legacy = json!({"enabled": false, "token": "legacy-token"});
        fs::write(home.join("mcp.json"), serde_json::to_vec(&legacy).unwrap()).unwrap();
        migrate_legacy_mcp_json(&conn, &home).unwrap();
        let stored: String = conn
            .query_row("SELECT value FROM settings WHERE key='mcp'", [], |r| {
                r.get(0)
            })
            .unwrap();
        assert_eq!(serde_json::from_str::<Value>(&stored).unwrap(), legacy);
        assert!(!home.join("mcp.json").exists());
        // 幂等: mcp.json 已不存在时不覆盖已有设置
        conn.execute(
            "UPDATE settings SET value='{\"enabled\":true,\"token\":\"changed\"}' WHERE key='mcp'",
            [],
        )
        .unwrap();
        migrate_legacy_mcp_json(&conn, &home).unwrap();
        let stored: String = conn
            .query_row("SELECT value FROM settings WHERE key='mcp'", [], |r| {
                r.get(0)
            })
            .unwrap();
        assert_eq!(
            serde_json::from_str::<Value>(&stored).unwrap(),
            json!({"enabled": true, "token": "changed"})
        );
        fs::remove_dir_all(home).unwrap();
    }

    #[test]
    fn migration_fails_on_malformed_legacy_mcp_json_and_keeps_it() {
        let home = test_dir("mcp-bad");
        let conn = Connection::open_in_memory().unwrap();
        initialize_schema(&conn).unwrap();
        fs::write(home.join("mcp.json"), r#"{"enabled":"yes"}"#).unwrap();
        assert!(migrate_legacy_mcp_json(&conn, &home).is_err());
        assert!(home.join("mcp.json").exists());
        fs::remove_dir_all(home).unwrap();
    }
}

fn row_profile(r: &rusqlite::Row<'_>) -> rusqlite::Result<Value> {
    Ok(
        json!({"id":r.get::<_,String>(0)?,"name":r.get::<_,String>(1)?,"uniqueId":r.get::<_,Option<String>>(2)?,"connectType":r.get::<_,String>(3)?,"command":r.get::<_,Option<String>>(4)?,"host":r.get::<_,String>(5)?,"port":r.get::<_,i64>(6)?,"username":r.get::<_,String>(7)?,"authType":r.get::<_,String>(8)?,"hasPassword":r.get::<_,Option<String>>(9)?.is_some(),"hasPrivateKey":r.get::<_,Option<String>>(10)?.is_some(),"term":r.get::<_,String>(11)?,"cols":r.get::<_,i64>(12)?,"rows":r.get::<_,i64>(13)?,"promptRegex":r.get::<_,Option<String>>(14)?,"createdAt":r.get::<_,i64>(15)?,"updatedAt":r.get::<_,i64>(16)?}),
    )
}
