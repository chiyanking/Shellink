use aes_gcm::{Aes256Gcm, KeyInit, Nonce, aead::{Aead, OsRng, rand_core::RngCore}};
use base64::{Engine as _, engine::general_purpose::STANDARD};
use rusqlite::{Connection, OptionalExtension};
use serde_json::{json, Value};
use sha2::{Digest, Sha256};
use std::{fs, path::PathBuf, sync::Mutex};

pub struct DataStore { conn: Mutex<Connection>, key: [u8; 32] }

impl DataStore {
    pub fn open() -> Result<Self, String> {
        let path = std::env::var_os("SHELLINK_DB").map(PathBuf::from).unwrap_or_else(|| {
            let home = std::env::var_os("HOME").map(PathBuf::from).unwrap_or_else(|| PathBuf::from("."));
            home.join(".Shellink").join("shellink.db")
        });
        let conn = Connection::open(&path).map_err(|e| format!("打开数据库失败: {e}"))?;
        conn.busy_timeout(std::time::Duration::from_secs(5)).map_err(|e| e.to_string())?;
        let key = load_master_key(&path)?;
        Ok(Self { conn: Mutex::new(conn), key })
    }
    pub fn profiles(&self, id: Option<&str>) -> Result<Value, String> {
        let c = self.conn.lock().unwrap();
        if let Some(id) = id {
            let mut s = c.prepare("SELECT id,name,unique_id,connect_type,command,host,port,username,auth_type,encrypted_password,encrypted_private_key,term,cols,rows,prompt_regex,created_at,updated_at FROM profiles WHERE id=?")
                .map_err(|e| e.to_string())?;
            let v = s.query_row([id], row_profile).optional().map_err(|e| e.to_string())?
                .ok_or_else(|| "Profile not found".to_string())?;
            return Ok(v);
        }
        let mut s = c.prepare("SELECT id,name,unique_id,connect_type,command,host,port,username,auth_type,encrypted_password,encrypted_private_key,term,cols,rows,prompt_regex,created_at,updated_at FROM profiles ORDER BY created_at").map_err(|e| e.to_string())?;
        let rows = s.query_map([], row_profile).map_err(|e| e.to_string())?;
        let mut out = Vec::new();
        for row in rows { out.push(row.map_err(|e| e.to_string())?); }
        Ok(Value::Array(out))
    }
    pub fn sessions(&self) -> Result<Value, String> {
        let c = self.conn.lock().unwrap();
        let mut s = c.prepare("SELECT id,profile_id,profile_name,target,state,mode,cols,rows,created_at,closed_at,close_reason,exit_code,name,source FROM sessions ORDER BY created_at").map_err(|e| e.to_string())?;
        let rows = s.query_map([], |r| Ok(json!({"id":r.get::<_,String>(0)?,"profileId":r.get::<_,String>(1)?,"profileName":r.get::<_,String>(2)?,"target":r.get::<_,String>(3)?,"state":r.get::<_,String>(4)?,"mode":r.get::<_,String>(5)?,"cols":r.get::<_,i64>(6)?,"rows":r.get::<_,i64>(7)?,"createdAt":r.get::<_,i64>(8)?,"closedAt":r.get::<_,Option<i64>>(9)?,"closeReason":r.get::<_,Option<String>>(10)?,"exitCode":r.get::<_,Option<i64>>(11)?,"name":r.get::<_,String>(12)?,"source":r.get::<_,String>(13)?,"active":r.get::<_,String>(4)? != "DISCONNECTED"}))).map_err(|e| e.to_string())?;
        let mut out=Vec::new(); for row in rows { out.push(row.map_err(|e|e.to_string())?); } Ok(Value::Array(out))
    }
    pub fn command_profile(&self, id: &str) -> Result<(String, String, String, i64, i64), String> {
        let c=self.conn.lock().unwrap(); c.query_row("SELECT name,connect_type,COALESCE(command,''),cols,rows FROM profiles WHERE id=?", [id], |r| Ok((r.get(0)?,r.get(1)?,r.get(2)?,r.get(3)?,r.get(4)?))).optional().map_err(|e|e.to_string())?.ok_or_else(||"Profile not found".into())
    }
    pub fn insert_session(&self, id: &str, profile_id: &str, profile_name: &str, target: &str, cols: i64, rows: i64, name: &str, source: &str, mode: &str) -> Result<(), String> {
        let c=self.conn.lock().unwrap(); c.execute("INSERT INTO sessions (id,profile_id,profile_name,target,state,mode,cols,rows,created_at,name,source) VALUES (?,?,?,?,?,?,?,?,?,?,?)", rusqlite::params![id,profile_id,profile_name,target,"CONNECTING",mode,cols,rows,now(),name,source]).map_err(|e|e.to_string()).map(|_|())
    }
    pub fn update_session_state(&self, id: &str, state: &str, reason: Option<&str>) -> Result<(), String> {
        let c=self.conn.lock().unwrap(); c.execute("UPDATE sessions SET state=?,closed_at=CASE WHEN ?='DISCONNECTED' THEN ? ELSE closed_at END,close_reason=CASE WHEN ?='DISCONNECTED' THEN ? ELSE close_reason END WHERE id=?", rusqlite::params![state,state,now(),state,reason,id]).map_err(|e|e.to_string()).map(|_|())
    }
    pub fn session_state(&self, id: &str) -> Result<Value, String> {
        let c=self.conn.lock().unwrap(); c.query_row("SELECT id,state,mode,closed_at,close_reason,exit_code FROM sessions WHERE id=?", [id], |r| Ok(json!({"id":r.get::<_,String>(0)?,"state":r.get::<_,String>(1)?,"mode":r.get::<_,String>(2)?,"closedAt":r.get::<_,Option<i64>>(3)?,"closeReason":r.get::<_,Option<String>>(4)?,"exitCode":r.get::<_,Option<i64>>(5)?,"active":r.get::<_,String>(1)? != "DISCONNECTED","lastSeq":0,"recentOutput":""}))).optional().map_err(|e|e.to_string())?.ok_or_else(||"Session not found".into())
    }
    pub fn add_output(&self, id: &str, text: &str) -> Result<(), String> {
        let c=self.conn.lock().unwrap(); let seq:i64=c.query_row("SELECT COALESCE(MAX(seq),0)+1 FROM history_chunks WHERE session_id=?", [id], |r|r.get(0)).map_err(|e|e.to_string())?; c.execute("INSERT INTO history_chunks (session_id,seq,direction,data_raw,data_plain,internal,created_at) VALUES (?,?,?,?,?,?,?)", rusqlite::params![id,seq,"output",text,text,0,now()]).map_err(|e|e.to_string()).map(|_|())
    }
    pub fn add_input(&self, id: &str, text: &str) -> Result<(), String> { let c=self.conn.lock().unwrap(); let seq:i64=c.query_row("SELECT COALESCE(MAX(seq),0)+1 FROM history_chunks WHERE session_id=?", [id], |r|r.get(0)).map_err(|e|e.to_string())?; c.execute("INSERT INTO history_chunks (session_id,seq,direction,data_raw,data_plain,internal,created_at) VALUES (?,?,?,?,?,?,?)", rusqlite::params![id,seq,"input",text,text,0,now()]).map_err(|e|e.to_string()).map(|_|()) }
    pub fn history_chunks(&self, id: &str, since: i64, limit: i64) -> Result<Value, String> { let c=self.conn.lock().unwrap(); let mut s=c.prepare("SELECT id,session_id,seq,direction,data_plain,created_at FROM history_chunks WHERE session_id=? AND seq>? AND internal=0 ORDER BY seq LIMIT ?").map_err(|e|e.to_string())?; let rows=s.query_map(rusqlite::params![id,since,limit],|r|Ok(json!({"id":r.get::<_,i64>(0)?,"sessionId":r.get::<_,String>(1)?,"seq":r.get::<_,i64>(2)?,"direction":r.get::<_,String>(3)?,"plain":r.get::<_,String>(4)?,"createdAt":r.get::<_,i64>(5)?}))).map_err(|e|e.to_string())?; let mut items=Vec::new();let mut cursor=since;for row in rows{let v=row.map_err(|e|e.to_string())?;cursor=v.get("seq").and_then(Value::as_i64).unwrap_or(cursor);items.push(v);}Ok(json!({"items":items,"cursor":cursor,"hasMore":items.len() as i64==limit})) }
    pub fn profile_create(&self, p: &Value) -> Result<Value, String> {
        let name = p.get("name").and_then(Value::as_str).filter(|s| !s.is_empty()).ok_or("name must be a non-empty string")?;
        let id = format!("rust-{}", now()); let t = now();
        let password = self.encrypt_field(p, "password")?;
        let private_key = self.encrypt_field(p, "privateKey")?;
        let passphrase = self.encrypt_field(p, "passphrase")?;
        let conn = self.conn.lock().unwrap();
        conn.execute("INSERT INTO profiles (id,name,unique_id,connect_type,command,host,port,username,auth_type,encrypted_password,encrypted_private_key,encrypted_passphrase,term,cols,rows,prompt_regex,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)",
            rusqlite::params![id,name,p.get("uniqueId").and_then(Value::as_str),p.get("connectType").and_then(Value::as_str).unwrap_or("ssh"),p.get("command").and_then(Value::as_str),p.get("host").and_then(Value::as_str).unwrap_or(""),p.get("port").and_then(Value::as_i64).unwrap_or(22),p.get("username").and_then(Value::as_str).unwrap_or(""),p.get("authType").and_then(Value::as_str).unwrap_or("password"),password,private_key,passphrase,p.get("term").and_then(Value::as_str).unwrap_or("xterm-256color"),p.get("cols").and_then(Value::as_i64).unwrap_or(160),p.get("rows").and_then(Value::as_i64).unwrap_or(42),p.get("promptRegex").and_then(Value::as_str),t,t]).map_err(|e| e.to_string())?;
        drop(conn); self.profiles(Some(&id))
    }
    pub fn profile_update(&self, id: &str, p: &Value) -> Result<Value, String> {
        let conn = self.conn.lock().unwrap();
        let changed = conn.execute("UPDATE profiles SET name=COALESCE(?,name),unique_id=COALESCE(?,unique_id),connect_type=COALESCE(?,connect_type),command=COALESCE(?,command),host=COALESCE(?,host),port=COALESCE(?,port),username=COALESCE(?,username),auth_type=COALESCE(?,auth_type),encrypted_password=CASE WHEN ? THEN ? ELSE encrypted_password END,encrypted_private_key=CASE WHEN ? THEN ? ELSE encrypted_private_key END,encrypted_passphrase=CASE WHEN ? THEN ? ELSE encrypted_passphrase END,term=COALESCE(?,term),cols=COALESCE(?,cols),rows=COALESCE(?,rows),prompt_regex=COALESCE(?,prompt_regex),updated_at=? WHERE id=?", rusqlite::params![p.get("name").and_then(Value::as_str),p.get("uniqueId").and_then(Value::as_str),p.get("connectType").and_then(Value::as_str),p.get("command").and_then(Value::as_str),p.get("host").and_then(Value::as_str),p.get("port").and_then(Value::as_i64),p.get("username").and_then(Value::as_str),p.get("authType").and_then(Value::as_str),p.get("password").is_some(),self.encrypt_field(p,"password")?,p.get("privateKey").is_some(),self.encrypt_field(p,"privateKey")?,p.get("passphrase").is_some(),self.encrypt_field(p,"passphrase")?,p.get("term").and_then(Value::as_str),p.get("cols").and_then(Value::as_i64),p.get("rows").and_then(Value::as_i64),p.get("promptRegex").and_then(Value::as_str),now(),id]).map_err(|e| e.to_string())?;
        if changed == 0 { return Err("Profile not found".into()); } drop(conn); self.profiles(Some(id))
    }
    fn encrypt_field(&self, p: &Value, name: &str) -> Result<Option<String>, String> {
        match p.get(name) { None | Some(Value::Null) => Ok(None), Some(v) => { let s=v.as_str().ok_or_else(|| format!("{name} must be a string"))?; if s.is_empty() { return Ok(None); } encrypt(&self.key, s).map(Some) } }
    }
    pub fn profile_delete(&self, id: &str) -> Result<Value, String> { let c=self.conn.lock().unwrap(); c.execute("DELETE FROM profiles WHERE id=?",[id]).map_err(|e|e.to_string())?; Ok(json!({"ok":true})) }
}

pub(crate) fn session_now() -> i64 { now() }
fn now() -> i64 { std::time::SystemTime::now().duration_since(std::time::UNIX_EPOCH).unwrap_or_default().as_millis() as i64 }

fn load_master_key(db: &PathBuf) -> Result<[u8; 32], String> {
    if let Ok(raw) = std::env::var("SHELLINK_MASTER_KEY") { return decode_key(&raw); }
    let home = db.parent().unwrap_or_else(|| std::path::Path::new("."));
    let file = home.join("master.key");
    if let Ok(raw) = fs::read_to_string(file) { return decode_key(raw.trim()); }
    let mut h = Sha256::new(); h.update(b"shellink-insecure-default-key"); Ok(h.finalize().into())
}
fn decode_key(raw: &str) -> Result<[u8; 32], String> { if raw.len()!=64 { return Err("master key must be 32 bytes".into()); } let mut out=[0u8;32]; for (i, pair) in raw.as_bytes().chunks_exact(2).enumerate() { out[i]=hex_nibble(pair[0]).and_then(|a|hex_nibble(pair[1]).map(|b|a*16+b)).ok_or("SHELLINK_MASTER_KEY must be hex")?; } Ok(out) }
fn hex_nibble(c: u8) -> Option<u8> { match c { b'0'..=b'9'=>Some(c-b'0'),b'a'..=b'f'=>Some(c-b'a'+10),b'A'..=b'F'=>Some(c-b'A'+10),_=>None } }
fn encrypt(key: &[u8; 32], plain: &str) -> Result<String, String> { let cipher=Aes256Gcm::new_from_slice(key).map_err(|e|e.to_string())?; let mut nonce=[0u8;12]; OsRng.fill_bytes(&mut nonce); let sealed=cipher.encrypt(Nonce::from_slice(&nonce),plain.as_bytes()).map_err(|e|e.to_string())?; let tag_at=sealed.len()-16; let mut out=Vec::with_capacity(sealed.len()+12); out.extend_from_slice(&nonce); out.extend_from_slice(&sealed[tag_at..]); out.extend_from_slice(&sealed[..tag_at]); Ok(STANDARD.encode(out)) }

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn decrypts_go_secretbox_vector() {
        let mut h = Sha256::new(); h.update(b"interop-key");
        let key: [u8; 32] = h.finalize().into();
        let raw = STANDARD.decode("BwcHBwcHBwcHBwcHuDosXAIoo7RWmWgXfiEk7bukxlTc1XCBE6MDCfY=").unwrap();
        let cipher = Aes256Gcm::new_from_slice(&key).unwrap();
        let nonce = Nonce::from_slice(&raw[..12]);
        let mut sealed = raw[28..].to_vec(); sealed.extend_from_slice(&raw[12..28]);
        assert_eq!(String::from_utf8(cipher.decrypt(nonce, sealed.as_ref()).unwrap()).unwrap(), "secret-中文");
    }
}

fn row_profile(r: &rusqlite::Row<'_>) -> rusqlite::Result<Value> {
    Ok(json!({"id":r.get::<_,String>(0)?,"name":r.get::<_,String>(1)?,"uniqueId":r.get::<_,Option<String>>(2)?,"connectType":r.get::<_,String>(3)?,"command":r.get::<_,Option<String>>(4)?,"host":r.get::<_,String>(5)?,"port":r.get::<_,i64>(6)?,"username":r.get::<_,String>(7)?,"authType":r.get::<_,String>(8)?,"hasPassword":r.get::<_,Option<String>>(9)?.is_some(),"hasPrivateKey":r.get::<_,Option<String>>(10)?.is_some(),"term":r.get::<_,String>(11)?,"cols":r.get::<_,i64>(12)?,"rows":r.get::<_,i64>(13)?,"promptRegex":r.get::<_,Option<String>>(14)?,"createdAt":r.get::<_,i64>(15)?,"updatedAt":r.get::<_,i64>(16)?}))
}
