use crate::data::DataStore;
use serde_json::json;
use std::{collections::HashMap, io::{Read, Write}, process::{Child, Command, Stdio}, sync::Mutex, thread};
use tauri::{AppHandle, Emitter, Manager};

struct Live { child: Child, input: std::process::ChildStdin }
pub struct SessionManager { live: Mutex<HashMap<String, Live>> }
impl SessionManager {
    pub fn new() -> Self { Self { live: Mutex::new(HashMap::new()) } }
    pub fn create(&self, app: &AppHandle, db: &DataStore, profile_id: &str, name: &str, source: &str, mode: &str, cols: i64, rows: i64) -> Result<serde_json::Value,String> {
        let (profile_name, kind, command, default_cols, default_rows)=db.command_profile(profile_id)?;
        if kind != "command" { return Err("Rust session currently supports command profiles only".into()); }
        if command.trim().is_empty() { return Err("Command profile has no command".into()); }
        let cols=if cols==0{default_cols}else{cols}; let rows=if rows==0{default_rows}else{rows};
        let id=format!("rust-session-{}", crate::data::session_now());
        let mut child=Command::new("/bin/sh").args(["-lc",&command]).stdin(Stdio::piped()).stdout(Stdio::piped()).stderr(Stdio::piped()).spawn().map_err(|e|e.to_string())?;
        let mut stdout=child.stdout.take().ok_or("无法打开 session 输出")?;
        let mut stderr=child.stderr.take().ok_or("无法打开 session 错误输出")?;
        let input=child.stdin.take().ok_or("无法打开 session 输入")?;
        db.insert_session(&id,profile_id,&profile_name,&command,cols,rows,name,source,mode)?;
        let event_app=app.clone(); let event_id=id.clone();
        thread::spawn(move || { let mut buf=[0u8;8192]; loop { match stdout.read(&mut buf) { Ok(0)|Err(_) => break, Ok(n) => emit_output(&event_app,&event_id,&buf[..n]) } } });
        let event_app=app.clone(); let event_id=id.clone();
        thread::spawn(move || { let mut buf=[0u8;8192]; loop { match stderr.read(&mut buf) { Ok(0)|Err(_) => break, Ok(n) => emit_output(&event_app,&event_id,&buf[..n]) } } });
        self.live.lock().unwrap().insert(id.clone(),Live{child,input});
        let event_app=app.clone(); let event_id=id.clone();
        thread::spawn(move || { let _=event_app.emit("shellink:event",json!({"event":"session.created","data":{"sessionId":event_id}})); });
        Ok(json!({"id":id,"state":"CONNECTING","mode":mode,"target":command}))
    }
    pub fn input(&self,id:&str,text:&str,append:bool)->Result<serde_json::Value,String>{let mut live=self.live.lock().unwrap();let s=live.get_mut(id).ok_or("Session not found")?;s.input.write_all(text.as_bytes()).map_err(|e|e.to_string())?;if append{s.input.write_all(b"\n").map_err(|e|e.to_string())?;}s.input.flush().map_err(|e|e.to_string())?;Ok(json!({"ok":true}))}
    pub fn close(&self,db:&DataStore,id:&str)->Result<serde_json::Value,String>{let mut live=self.live.lock().unwrap();if let Some(mut s)=live.remove(id){let _=s.child.kill();let _=s.child.wait();}db.update_session_state(id,"DISCONNECTED",Some("Closed by user"))?;Ok(json!({"closed":true}))}
    pub fn shutdown(&self,db:&DataStore){let ids:Vec<String>=self.live.lock().unwrap().keys().cloned().collect();for id in ids{let _=self.close(db,&id);}}
}
fn emit_output(app:&AppHandle,id:&str,bytes:&[u8]){let text=String::from_utf8_lossy(bytes).into_owned();if let Some(db)=app.try_state::<Option<DataStore>>() { if let Some(db)=db.inner().as_ref() { let _=db.add_output(id,&text); } }let _=app.emit("shellink:event",json!({"event":"session.output","data":{"sessionId":id,"direction":"output","raw":text,"plain":text}}));}
