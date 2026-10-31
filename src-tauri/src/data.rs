use rusqlite::{Connection, OptionalExtension};
use serde_json::{json, Value};
use std::{path::PathBuf, sync::Mutex};

pub struct DataStore { conn: Mutex<Connection>, path: PathBuf }

impl DataStore {
    pub fn open() -> Result<Self, String> {
        let path = std::env::var_os("SHELLINK_DB").map(PathBuf::from).unwrap_or_else(|| {
            let home = std::env::var_os("HOME").map(PathBuf::from).unwrap_or_else(|| PathBuf::from("."));
            home.join(".Shellink").join("shellink.db")
        });
        let conn = Connection::open(&path).map_err(|e| format!("打开数据库失败: {e}"))?;
        conn.busy_timeout(std::time::Duration::from_secs(5)).map_err(|e| e.to_string())?;
        Ok(Self { conn: Mutex::new(conn), path })
    }
    pub fn path(&self) -> &PathBuf { &self.path }
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
    pub fn profile_create(&self, p: &Value) -> Result<Value, String> {
        if p.get("password").is_some() || p.get("privateKey").is_some() || p.get("passphrase").is_some() { return Err("凭证写入仍由迁移桥接处理".into()); }
        let name = p.get("name").and_then(Value::as_str).filter(|s| !s.is_empty()).ok_or("name must be a non-empty string")?;
        let id = format!("rust-{}", now()); let t = now();
        let conn = self.conn.lock().unwrap();
        conn.execute("INSERT INTO profiles (id,name,unique_id,connect_type,command,host,port,username,auth_type,term,cols,rows,prompt_regex,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)",
            rusqlite::params![id,name,p.get("uniqueId").and_then(Value::as_str),p.get("connectType").and_then(Value::as_str).unwrap_or("ssh"),p.get("command").and_then(Value::as_str),p.get("host").and_then(Value::as_str).unwrap_or(""),p.get("port").and_then(Value::as_i64).unwrap_or(22),p.get("username").and_then(Value::as_str).unwrap_or(""),p.get("authType").and_then(Value::as_str).unwrap_or("password"),p.get("term").and_then(Value::as_str).unwrap_or("xterm-256color"),p.get("cols").and_then(Value::as_i64).unwrap_or(160),p.get("rows").and_then(Value::as_i64).unwrap_or(42),p.get("promptRegex").and_then(Value::as_str),t,t]).map_err(|e| e.to_string())?;
        drop(conn); self.profiles(Some(&id))
    }
    pub fn profile_update(&self, id: &str, p: &Value) -> Result<Value, String> {
        if p.get("password").is_some() || p.get("privateKey").is_some() || p.get("passphrase").is_some() { return Err("凭证写入仍由迁移桥接处理".into()); }
        let conn = self.conn.lock().unwrap();
        let changed = conn.execute("UPDATE profiles SET name=COALESCE(?,name),unique_id=COALESCE(?,unique_id),connect_type=COALESCE(?,connect_type),command=COALESCE(?,command),host=COALESCE(?,host),port=COALESCE(?,port),username=COALESCE(?,username),auth_type=COALESCE(?,auth_type),term=COALESCE(?,term),cols=COALESCE(?,cols),rows=COALESCE(?,rows),prompt_regex=COALESCE(?,prompt_regex),updated_at=? WHERE id=?", rusqlite::params![p.get("name").and_then(Value::as_str),p.get("uniqueId").and_then(Value::as_str),p.get("connectType").and_then(Value::as_str),p.get("command").and_then(Value::as_str),p.get("host").and_then(Value::as_str),p.get("port").and_then(Value::as_i64),p.get("username").and_then(Value::as_str),p.get("authType").and_then(Value::as_str),p.get("term").and_then(Value::as_str),p.get("cols").and_then(Value::as_i64),p.get("rows").and_then(Value::as_i64),p.get("promptRegex").and_then(Value::as_str),now(),id]).map_err(|e| e.to_string())?;
        if changed == 0 { return Err("Profile not found".into()); } drop(conn); self.profiles(Some(id))
    }
    pub fn profile_delete(&self, id: &str) -> Result<Value, String> { let c=self.conn.lock().unwrap(); c.execute("DELETE FROM profiles WHERE id=?",[id]).map_err(|e|e.to_string())?; Ok(json!({"ok":true})) }
}

fn now() -> i64 { std::time::SystemTime::now().duration_since(std::time::UNIX_EPOCH).unwrap_or_default().as_millis() as i64 }

fn row_profile(r: &rusqlite::Row<'_>) -> rusqlite::Result<Value> {
    Ok(json!({"id":r.get::<_,String>(0)?,"name":r.get::<_,String>(1)?,"uniqueId":r.get::<_,Option<String>>(2)?,"connectType":r.get::<_,String>(3)?,"command":r.get::<_,Option<String>>(4)?,"host":r.get::<_,String>(5)?,"port":r.get::<_,i64>(6)?,"username":r.get::<_,String>(7)?,"authType":r.get::<_,String>(8)?,"hasPassword":r.get::<_,Option<String>>(9)?.is_some(),"hasPrivateKey":r.get::<_,Option<String>>(10)?.is_some(),"term":r.get::<_,String>(11)?,"cols":r.get::<_,i64>(12)?,"rows":r.get::<_,i64>(13)?,"promptRegex":r.get::<_,Option<String>>(14)?,"createdAt":r.get::<_,i64>(15)?,"updatedAt":r.get::<_,i64>(16)?}))
}
