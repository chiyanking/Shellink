use aes_gcm::{
    Aes256Gcm, KeyInit, Nonce,
    aead::{Aead, OsRng, rand_core::RngCore},
};
use base64::{Engine as _, engine::general_purpose::STANDARD};
use rusqlite::{Connection, OptionalExtension};
use serde_json::{Value, json};
#[cfg(test)]
use sha2::{Digest, Sha256};
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

impl DataStore {
    pub fn open() -> Result<Self, String> {
        let data_home = resolve_data_home()?;
        ensure_private_dir(&data_home)?;
        let path = std::env::var_os("SHELLINK_DB")
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
        migrate(&conn)?;
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
    pub fn command_profile(&self, id: &str) -> Result<(String, String, String, i64, i64), String> {
        let c = self.conn.lock().unwrap();
        c.query_row(
            "SELECT name,connect_type,COALESCE(command,''),cols,rows FROM profiles WHERE id=?",
            [id],
            |r| Ok((r.get(0)?, r.get(1)?, r.get(2)?, r.get(3)?, r.get(4)?)),
        )
        .optional()
        .map_err(|e| e.to_string())?
        .ok_or_else(|| "Profile not found".into())
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
    pub fn session_state(&self, id: &str) -> Result<Value, String> {
        let c = self.conn.lock().unwrap();
        c.query_row("SELECT id,state,mode,closed_at,close_reason,exit_code FROM sessions WHERE id=?", [id], |r| Ok(json!({"id":r.get::<_,String>(0)?,"state":r.get::<_,String>(1)?,"mode":r.get::<_,String>(2)?,"closedAt":r.get::<_,Option<i64>>(3)?,"closeReason":r.get::<_,Option<String>>(4)?,"exitCode":r.get::<_,Option<i64>>(5)?,"active":r.get::<_,String>(1)? != "DISCONNECTED","lastSeq":0,"recentOutput":""}))).optional().map_err(|e|e.to_string())?.ok_or_else(||"Session not found".into())
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
        let mut state = json!({"enabled":true,"token":""});
        if let Ok(raw) = std::fs::read_to_string(&path) {
            state = serde_json::from_str(&raw).map_err(|e| e.to_string())?;
        }
        if state
            .get("token")
            .and_then(Value::as_str)
            .unwrap_or("")
            .is_empty()
        {
            state["token"] = Value::String(random_token());
            std::fs::create_dir_all(&self.data_home).map_err(|e| e.to_string())?;
            std::fs::write(
                &path,
                serde_json::to_vec_pretty(&state).map_err(|e| e.to_string())?,
            )
            .map_err(|e| e.to_string())?;
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
        Ok(
            json!({"enabled":enabled,"running":false,"httpEnabled":enabled,"url":url,"token":token,"clientConfig":{"mcpServers":{"shellink":{"url":url,"headers":{"Authorization":format!("Bearer {}",token)}}}}}),
        )
    }
    pub fn mcp_config(&self, params: &Value) -> Result<Value, String> {
        let path = self.data_home.join("mcp.json");
        let mut state = json!({"enabled":true,"token":random_token()});
        if let Ok(raw) = std::fs::read_to_string(&path) {
            state = serde_json::from_str(&raw).map_err(|e| e.to_string())?;
        }
        if let Some(enabled) = params.get("enabled").and_then(Value::as_bool) {
            state["enabled"] = Value::Bool(enabled);
        }
        if params
            .get("regenerateToken")
            .and_then(Value::as_bool)
            .unwrap_or(false)
        {
            state["token"] = Value::String(random_token());
        }
        std::fs::create_dir_all(&self.data_home).map_err(|e| e.to_string())?;
        std::fs::write(
            &path,
            serde_json::to_vec_pretty(&state).map_err(|e| e.to_string())?,
        )
        .map_err(|e| e.to_string())?;
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
        c.execute("DELETE FROM profiles WHERE id=?", [id])
            .map_err(|e| e.to_string())?;
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
    let explicit = std::env::var_os("SHELLINK_HOME").map(PathBuf::from);
    let appdata = std::env::var_os("APPDATA").map(PathBuf::from);
    let home = std::env::var_os("HOME").map(PathBuf::from);
    resolve_data_home_from(explicit, appdata, home, cfg!(windows))
}
fn resolve_data_home_from(
    explicit: Option<PathBuf>,
    appdata: Option<PathBuf>,
    home: Option<PathBuf>,
    windows: bool,
) -> Result<PathBuf, String> {
    let path = if let Some(path) = explicit {
        path
    } else if windows {
        appdata
            .or_else(|| home.map(|path| path.join("AppData").join("Roaming")))
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
fn migrate(conn: &Connection) -> Result<(), String> {
    conn.execute_batch("CREATE TABLE IF NOT EXISTS profiles (id TEXT PRIMARY KEY,name TEXT NOT NULL,unique_id TEXT,connect_type TEXT NOT NULL DEFAULT 'ssh',command TEXT,host TEXT NOT NULL DEFAULT '',port INTEGER NOT NULL DEFAULT 22,username TEXT NOT NULL DEFAULT '',auth_type TEXT NOT NULL DEFAULT 'password',encrypted_password TEXT,encrypted_private_key TEXT,encrypted_passphrase TEXT,term TEXT NOT NULL DEFAULT 'xterm-256color',cols INTEGER NOT NULL DEFAULT 160,rows INTEGER NOT NULL DEFAULT 42,prompt_regex TEXT,created_at INTEGER NOT NULL,updated_at INTEGER NOT NULL); CREATE TABLE IF NOT EXISTS sessions (id TEXT PRIMARY KEY,profile_id TEXT NOT NULL,profile_name TEXT NOT NULL,target TEXT NOT NULL,state TEXT NOT NULL,mode TEXT NOT NULL DEFAULT 'AUTO',cols INTEGER NOT NULL,rows INTEGER NOT NULL,created_at INTEGER NOT NULL,closed_at INTEGER,close_reason TEXT,exit_code INTEGER); CREATE TABLE IF NOT EXISTS history_chunks (id INTEGER PRIMARY KEY AUTOINCREMENT,session_id TEXT NOT NULL,seq INTEGER NOT NULL,direction TEXT NOT NULL,data_raw TEXT NOT NULL,data_plain TEXT NOT NULL,internal INTEGER NOT NULL DEFAULT 0,created_at INTEGER NOT NULL); CREATE TABLE IF NOT EXISTS webhooks (id TEXT PRIMARY KEY,url TEXT NOT NULL,events TEXT NOT NULL DEFAULT '[]',created_at INTEGER NOT NULL); CREATE INDEX IF NOT EXISTS idx_history_session_seq ON history_chunks(session_id,seq);").map_err(|e|e.to_string())?;
    for (table, column, def) in [
        ("profiles", "unique_id", "TEXT"),
        ("sessions", "name", "TEXT NOT NULL DEFAULT ''"),
        ("sessions", "source", "TEXT NOT NULL DEFAULT 'unknown'"),
        ("history_chunks", "internal", "INTEGER NOT NULL DEFAULT 0"),
    ] {
        let mut s = conn
            .prepare(&format!("PRAGMA table_info({table})"))
            .map_err(|e| e.to_string())?;
        let names = s
            .query_map([], |r| r.get::<_, String>(1))
            .map_err(|e| e.to_string())?
            .collect::<Result<Vec<_>, _>>()
            .map_err(|e| e.to_string())?;
        drop(s);
        if !names.iter().any(|n| n == column) {
            conn.execute(
                &format!("ALTER TABLE {table} ADD COLUMN {column} {def}"),
                [],
            )
            .map_err(|e| e.to_string())?;
        }
    }
    conn.execute_batch("CREATE UNIQUE INDEX IF NOT EXISTS idx_profiles_unique_id ON profiles(unique_id) WHERE unique_id IS NOT NULL;").map_err(|e|e.to_string())
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
    fn decrypts_go_secretbox_vector() {
        let mut h = Sha256::new();
        h.update(b"interop-key");
        let key: [u8; 32] = h.finalize().into();
        let raw = STANDARD
            .decode("BwcHBwcHBwcHBwcHuDosXAIoo7RWmWgXfiEk7bukxlTc1XCBE6MDCfY=")
            .unwrap();
        let cipher = Aes256Gcm::new_from_slice(&key).unwrap();
        let nonce = Nonce::from_slice(&raw[..12]);
        let mut sealed = raw[28..].to_vec();
        sealed.extend_from_slice(&raw[12..28]);
        assert_eq!(
            String::from_utf8(cipher.decrypt(nonce, sealed.as_ref()).unwrap()).unwrap(),
            "secret-中文"
        );
    }

    #[test]
    fn resolves_data_home_using_go_platform_conventions() {
        let home = PathBuf::from("/users/shellink");
        assert_eq!(
            resolve_data_home_from(None, None, Some(home.clone()), false).unwrap(),
            home.join(".Shellink")
        );
        assert_eq!(
            resolve_data_home_from(
                None,
                Some(PathBuf::from("/appdata")),
                Some(home.clone()),
                true
            )
            .unwrap(),
            PathBuf::from("/appdata/Shellink")
        );
        assert_eq!(
            resolve_data_home_from(Some(PathBuf::from("/custom")), None, Some(home), true).unwrap(),
            PathBuf::from("/custom")
        );
    }

    #[test]
    fn migrates_legacy_database_columns_before_creating_indexes() {
        let conn = Connection::open_in_memory().unwrap();
        conn.execute_batch("CREATE TABLE profiles (id TEXT PRIMARY KEY,name TEXT NOT NULL,connect_type TEXT NOT NULL,command TEXT,host TEXT NOT NULL,port INTEGER NOT NULL,username TEXT NOT NULL,auth_type TEXT NOT NULL,encrypted_password TEXT,encrypted_private_key TEXT,encrypted_passphrase TEXT,term TEXT NOT NULL,cols INTEGER NOT NULL,rows INTEGER NOT NULL,prompt_regex TEXT,created_at INTEGER NOT NULL,updated_at INTEGER NOT NULL); CREATE TABLE sessions (id TEXT PRIMARY KEY,profile_id TEXT NOT NULL,profile_name TEXT NOT NULL,target TEXT NOT NULL,state TEXT NOT NULL,mode TEXT NOT NULL,cols INTEGER NOT NULL,rows INTEGER NOT NULL,created_at INTEGER NOT NULL,closed_at INTEGER,close_reason TEXT,exit_code INTEGER); CREATE TABLE history_chunks (id INTEGER PRIMARY KEY AUTOINCREMENT,session_id TEXT NOT NULL,seq INTEGER NOT NULL,direction TEXT NOT NULL,data_raw TEXT NOT NULL,data_plain TEXT NOT NULL,created_at INTEGER NOT NULL);").unwrap();

        migrate(&conn).unwrap();

        for (table, column) in [
            ("profiles", "unique_id"),
            ("sessions", "name"),
            ("sessions", "source"),
            ("history_chunks", "internal"),
        ] {
            let found: bool = conn.query_row(&format!("SELECT EXISTS(SELECT 1 FROM pragma_table_info('{table}') WHERE name='{column}')"), [], |row| row.get(0)).unwrap();
            assert!(found, "missing {table}.{column}");
        }
        let index_exists: bool = conn.query_row("SELECT EXISTS(SELECT 1 FROM sqlite_master WHERE type='index' AND name='idx_profiles_unique_id')", [], |row| row.get(0)).unwrap();
        assert!(index_exists);
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
}

fn row_profile(r: &rusqlite::Row<'_>) -> rusqlite::Result<Value> {
    Ok(
        json!({"id":r.get::<_,String>(0)?,"name":r.get::<_,String>(1)?,"uniqueId":r.get::<_,Option<String>>(2)?,"connectType":r.get::<_,String>(3)?,"command":r.get::<_,Option<String>>(4)?,"host":r.get::<_,String>(5)?,"port":r.get::<_,i64>(6)?,"username":r.get::<_,String>(7)?,"authType":r.get::<_,String>(8)?,"hasPassword":r.get::<_,Option<String>>(9)?.is_some(),"hasPrivateKey":r.get::<_,Option<String>>(10)?.is_some(),"term":r.get::<_,String>(11)?,"cols":r.get::<_,i64>(12)?,"rows":r.get::<_,i64>(13)?,"promptRegex":r.get::<_,Option<String>>(14)?,"createdAt":r.get::<_,i64>(15)?,"updatedAt":r.get::<_,i64>(16)?}),
    )
}
