use aes_gcm::{
    Aes256Gcm, KeyInit, Nonce,
    aead::{Aead, OsRng, rand_core::RngCore},
};
use base64::{Engine as _, engine::general_purpose::STANDARD};
use rusqlite::{Connection, OptionalExtension};
use serde_json::{Value, json};
use std::{
    fs,
    io::Write,
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
            "PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON; PRAGMA busy_timeout=5000;",
        )
        .map_err(|e| e.to_string())?;
        initialize_schema(&conn)?;
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
        let mut s = c.prepare("SELECT id,profile_id,profile_name,target,state,mode,cols,rows,created_at,closed_at,close_reason,exit_code,name,source FROM sessions ORDER BY created_at").map_err(|e| e.to_string())?;
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
    pub fn session_state(&self, id: &str) -> Result<Value, String> {
        let c = self.conn.lock().unwrap();
        c.query_row("SELECT id,state,mode,closed_at,close_reason,exit_code,(SELECT COALESCE(MAX(seq),0) FROM history_chunks WHERE session_id=sessions.id),(SELECT COALESCE(group_concat(data_plain,''),'') FROM (SELECT data_plain FROM history_chunks WHERE session_id=sessions.id AND direction='output' ORDER BY seq DESC LIMIT 15)) FROM sessions WHERE id=?", [id], |r| {
            let recent: String = r.get(7)?;
            Ok(json!({"id":r.get::<_,String>(0)?,"state":r.get::<_,String>(1)?,"mode":r.get::<_,String>(2)?,"closedAt":r.get::<_,Option<i64>>(3)?,"closeReason":r.get::<_,Option<String>>(4)?,"exitCode":r.get::<_,Option<i64>>(5)?,"active":r.get::<_,String>(1)? != "DISCONNECTED","lastSeq":r.get::<_,i64>(6)?,"recentOutput":recent}))
        }).optional().map_err(|e|e.to_string())?.ok_or_else(||"Session not found".into())
    }
    pub fn add_output(&self, id: &str, text: &str) -> Result<(), String> {
        let c = self.conn.lock().unwrap();
        let seq: i64 = c
            .query_row(
                "SELECT COALESCE(MAX(seq),0)+1 FROM history_chunks WHERE session_id=?",
                [id],
                |r| r.get(0),
            )
            .map_err(|e| e.to_string())?;
        c.execute("INSERT INTO history_chunks (session_id,seq,direction,data_raw,data_plain,internal,created_at) VALUES (?,?,?,?,?,?,?)", rusqlite::params![id,seq,"output",text,text,0,now()]).map_err(|e|e.to_string()).map(|_|())
    }
    pub fn mcp_info(&self) -> Result<Value, String> {
        let path = self.data_home.join("mcp.json");
        let mut state = load_mcp_state(&path)?;
        if state
            .get("token")
            .and_then(Value::as_str)
            .unwrap_or("")
            .is_empty()
        {
            state["token"] = Value::String(random_token());
            save_mcp_state(&path, &state)?;
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
        Ok(json!({
            "enabled":enabled,
            "running":false,
            "httpEnabled":enabled,
            "bindHost":std::env::var("SHELLINK_HOST").unwrap_or_else(|_| "127.0.0.1".into()),
            "url":url,
            "token":token,
            "command":command,
            "args":["mcp"],
            "clientConfig":{"mcpServers":{"shellink":{"url":url,"headers":{"Authorization":format!("Bearer {}",token)}}}},
            "clientConfigStdio":{"mcpServers":{"shellink":{"command":command,"args":["mcp"]}}}
        }))
    }
    pub fn mcp_config(&self, params: &Value) -> Result<Value, String> {
        let path = self.data_home.join("mcp.json");
        let mut state = load_mcp_state(&path)?;
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
        save_mcp_state(&path, &state)?;
        self.mcp_info()
    }
    pub fn add_input(&self, id: &str, text: &str) -> Result<(), String> {
        let c = self.conn.lock().unwrap();
        let seq: i64 = c
            .query_row(
                "SELECT COALESCE(MAX(seq),0)+1 FROM history_chunks WHERE session_id=?",
                [id],
                |r| r.get(0),
            )
            .map_err(|e| e.to_string())?;
        c.execute("INSERT INTO history_chunks (session_id,seq,direction,data_raw,data_plain,internal,created_at) VALUES (?,?,?,?,?,?,?)", rusqlite::params![id,seq,"input",text,text,0,now()]).map_err(|e|e.to_string()).map(|_|())
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
            .query_row("SELECT EXISTS(SELECT 1 FROM sessions WHERE id=?)", [id], |r| r.get(0))
            .map_err(|e| e.to_string())?;
        if !exists {
            return Err("Session not found".into());
        }
        let mut statement = c.prepare("SELECT data_raw FROM history_chunks WHERE session_id=? AND direction='output' ORDER BY seq").map_err(|e| e.to_string())?;
        let rows = statement
            .query_map([id], |row| row.get::<_, String>(0))
            .map_err(|e| e.to_string())?;
        let mut raw = String::new();
        let mut cursor = 0;
        for row in rows {
            raw.push_str(&row.map_err(|e| e.to_string())?);
            cursor += 1;
        }
        Ok(json!({"raw":raw,"cursor":cursor}))
    }
    pub fn webhooks(&self) -> Result<Value, String> {
        let c = self.conn.lock().unwrap();
        let mut statement = c
            .prepare("SELECT id,url,events,created_at FROM webhooks ORDER BY created_at")
            .map_err(|e| e.to_string())?;
        let rows = statement.query_map([], |r| {
            let events = serde_json::from_str::<Value>(&r.get::<_, String>(2)?).unwrap_or_else(|_| json!([]));
            Ok(json!({"id":r.get::<_,String>(0)?,"url":r.get::<_,String>(1)?,"events":events,"createdAt":r.get::<_,i64>(3)?}))
        }).map_err(|e| e.to_string())?;
        rows.map(|row| row.map_err(|e| e.to_string()).map(|v| v))
            .collect()
    }
    pub fn webhook_create(&self, params: &Value) -> Result<Value, String> {
        let url = params
            .get("url")
            .and_then(Value::as_str)
            .filter(|v| v.starts_with("http://") || v.starts_with("https://"))
            .ok_or("url must be a valid HTTP or HTTPS URL")?;
        if url.contains(char::is_whitespace) {
            return Err("url must be a valid HTTP or HTTPS URL".into());
        }
        let events = params.get("events").cloned().unwrap_or_else(|| json!([]));
        let Some(items) = events.as_array() else {
            return Err("events must be an array".into());
        };
        if items.iter().any(|item| {
            !matches!(
                item.as_str(),
                Some("state") | Some("closed") | Some("loginExternal")
            )
        }) {
            return Err("events must contain state, closed, or loginExternal".into());
        }
        let id = format!("webhook-{}", now());
        let created = now();
        let c = self.conn.lock().unwrap();
        c.execute(
            "INSERT INTO webhooks (id,url,events,created_at) VALUES (?,?,?,?)",
            rusqlite::params![id, url, events.to_string(), created],
        )
        .map_err(|e| e.to_string())?;
        Ok(json!({"id":id,"url":url,"events":events,"createdAt":created}))
    }
    pub fn webhook_delete(&self, id: &str) -> Result<Value, String> {
        let c = self.conn.lock().unwrap();
        if c.execute("DELETE FROM webhooks WHERE id=?", [id])
            .map_err(|e| e.to_string())?
            == 0
        {
            return Err("Webhook not found".into());
        }
        Ok(json!({"ok":true}))
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
            .map_err(|e| e.to_string())? == 0
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
    conn.execute_batch("CREATE TABLE IF NOT EXISTS profiles (id TEXT PRIMARY KEY,name TEXT NOT NULL,unique_id TEXT,connect_type TEXT NOT NULL DEFAULT 'ssh',command TEXT,host TEXT NOT NULL DEFAULT '',port INTEGER NOT NULL DEFAULT 22,username TEXT NOT NULL DEFAULT '',auth_type TEXT NOT NULL DEFAULT 'password',encrypted_password TEXT,encrypted_private_key TEXT,encrypted_passphrase TEXT,term TEXT NOT NULL DEFAULT 'xterm-256color',cols INTEGER NOT NULL DEFAULT 160,rows INTEGER NOT NULL DEFAULT 42,prompt_regex TEXT,created_at INTEGER NOT NULL,updated_at INTEGER NOT NULL); CREATE TABLE IF NOT EXISTS sessions (id TEXT PRIMARY KEY,profile_id TEXT NOT NULL,profile_name TEXT NOT NULL,target TEXT NOT NULL,state TEXT NOT NULL,mode TEXT NOT NULL DEFAULT 'AUTO',cols INTEGER NOT NULL,rows INTEGER NOT NULL,created_at INTEGER NOT NULL,closed_at INTEGER,close_reason TEXT,exit_code INTEGER,name TEXT NOT NULL DEFAULT '',source TEXT NOT NULL DEFAULT 'unknown'); CREATE TABLE IF NOT EXISTS history_chunks (id INTEGER PRIMARY KEY AUTOINCREMENT,session_id TEXT NOT NULL,seq INTEGER NOT NULL,direction TEXT NOT NULL,data_raw TEXT NOT NULL,data_plain TEXT NOT NULL,internal INTEGER NOT NULL DEFAULT 0,created_at INTEGER NOT NULL); CREATE TABLE IF NOT EXISTS webhooks (id TEXT PRIMARY KEY,url TEXT NOT NULL,events TEXT NOT NULL DEFAULT '[]',created_at INTEGER NOT NULL); CREATE UNIQUE INDEX IF NOT EXISTS idx_profiles_unique_id ON profiles(unique_id) WHERE unique_id IS NOT NULL; CREATE INDEX IF NOT EXISTS idx_history_session_seq ON history_chunks(session_id,seq);").map_err(|e|e.to_string())
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

fn load_mcp_state(path: &Path) -> Result<Value, String> {
    let raw = match fs::read_to_string(path) {
        Ok(raw) => raw,
        Err(error) if error.kind() == std::io::ErrorKind::NotFound => {
            return Ok(json!({"enabled": true, "token": ""}));
        }
        Err(error) => return Err(format!("读取 MCP 配置失败: {error}")),
    };
    let state: Value =
        serde_json::from_str(&raw).map_err(|error| format!("解析 MCP 配置失败: {error}"))?;
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
    Ok(state)
}

fn save_mcp_state(path: &Path, state: &Value) -> Result<(), String> {
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
    if let Some(parent) = path.parent() {
        ensure_private_dir(parent)?;
    }
    let temp = path.with_extension(format!("json.{}.tmp", std::process::id()));
    let bytes = serde_json::to_vec_pretty(state)
        .map_err(|error| format!("序列化 MCP 配置失败: {error}"))?;
    {
        let mut options = fs::OpenOptions::new();
        options.write(true).create(true).truncate(true);
        #[cfg(unix)]
        {
            use std::os::unix::fs::OpenOptionsExt;
            options.mode(0o600);
        }
        let mut file = options
            .open(&temp)
            .map_err(|error| format!("创建 MCP 配置临时文件失败: {error}"))?;
        if let Err(error) = file.write_all(&bytes).and_then(|_| file.sync_all()) {
            let _ = fs::remove_file(&temp);
            return Err(format!("写入 MCP 配置失败: {error}"));
        }
    }
    if let Err(error) = fs::rename(&temp, path) {
        let _ = fs::remove_file(&temp);
        return Err(format!("保存 MCP 配置失败: {error}"));
    }
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        let mut permissions = fs::metadata(path)
            .map_err(|error| format!("读取 MCP 配置权限失败: {error}"))?
            .permissions();
        permissions.set_mode(0o600);
        fs::set_permissions(path, permissions)
            .map_err(|error| format!("设置 MCP 配置权限失败: {error}"))?;
    }
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
    fn mcp_state_defaults_validates_and_round_trips() {
        let home = test_dir("mcp-state");
        let path = home.join("mcp.json");
        assert_eq!(
            load_mcp_state(&path).unwrap(),
            json!({"enabled": true, "token": ""})
        );
        save_mcp_state(&path, &json!({"enabled": false, "token": "test-token"})).unwrap();
        assert_eq!(
            load_mcp_state(&path).unwrap(),
            json!({"enabled": false, "token": "test-token"})
        );
        fs::write(&path, "[]").unwrap();
        assert!(load_mcp_state(&path).is_err());
        fs::write(&path, r#"{"enabled":"yes"}"#).unwrap();
        assert!(load_mcp_state(&path).is_err());
        fs::remove_dir_all(home).unwrap();
    }
}

fn row_profile(r: &rusqlite::Row<'_>) -> rusqlite::Result<Value> {
    Ok(
        json!({"id":r.get::<_,String>(0)?,"name":r.get::<_,String>(1)?,"uniqueId":r.get::<_,Option<String>>(2)?,"connectType":r.get::<_,String>(3)?,"command":r.get::<_,Option<String>>(4)?,"host":r.get::<_,String>(5)?,"port":r.get::<_,i64>(6)?,"username":r.get::<_,String>(7)?,"authType":r.get::<_,String>(8)?,"hasPassword":r.get::<_,Option<String>>(9)?.is_some(),"hasPrivateKey":r.get::<_,Option<String>>(10)?.is_some(),"term":r.get::<_,String>(11)?,"cols":r.get::<_,i64>(12)?,"rows":r.get::<_,i64>(13)?,"promptRegex":r.get::<_,Option<String>>(14)?,"createdAt":r.get::<_,i64>(15)?,"updatedAt":r.get::<_,i64>(16)?}),
    )
}
