use serde_json::{Value, json};
use std::{
    io::{BufRead, BufReader, Read, Write},
    net::{TcpListener, TcpStream},
    sync::{
        Arc, Mutex,
        atomic::{AtomicBool, Ordering},
        mpsc,
    },
    thread,
    time::{Duration, Instant},
};

const AUTH_TIMEOUT: Duration = Duration::from_secs(300);
const MAX_MESSAGE: u64 = 65_536;

pub struct Credential {
    pub secret: String,
    pub is_key: bool,
}

struct Pending {
    id: u64,
    prompt: Value,
    response: mpsc::Sender<String>,
}

pub struct SshAuth {
    pub address: String,
    pub token: String,
    pending: Arc<Mutex<Option<Pending>>>,
    stopped: Arc<AtomicBool>,
}

impl SshAuth {
    pub fn start(
        mut credential: Option<Credential>,
        changed: impl Fn() + Send + 'static,
    ) -> Result<Self, String> {
        let listener = TcpListener::bind("127.0.0.1:0").map_err(|e| e.to_string())?;
        listener.set_nonblocking(true).map_err(|e| e.to_string())?;
        let address = listener
            .local_addr()
            .map_err(|e| e.to_string())?
            .to_string();
        let token = rand::random::<[u8; 32]>()
            .iter()
            .map(|b| format!("{b:02x}"))
            .collect::<String>();
        let pending = Arc::new(Mutex::new(None));
        let stopped = Arc::new(AtomicBool::new(false));
        let auth = Self {
            address,
            token: token.clone(),
            pending: pending.clone(),
            stopped: stopped.clone(),
        };
        thread::spawn(move || {
            let mut next_id = 0;
            while !stopped.load(Ordering::Acquire) {
                let (mut stream, _) = match listener.accept() {
                    Ok(pair) => pair,
                    Err(e) if e.kind() == std::io::ErrorKind::WouldBlock => {
                        thread::sleep(Duration::from_millis(25));
                        continue;
                    }
                    Err(_) => break,
                };
                let _ = stream.set_read_timeout(Some(Duration::from_secs(1)));
                let _ = stream.set_write_timeout(Some(Duration::from_secs(1)));
                let Ok(request) = read_message(&stream) else {
                    continue;
                };
                if request["token"].as_str() != Some(token.as_str()) {
                    continue;
                }
                let Some(prompt) = request["prompt"].as_str() else {
                    continue;
                };
                let hint = request["hint"].as_str().unwrap_or("");
                let automatic = credential.as_ref().is_some_and(|saved| {
                    hint.is_empty() && matches_credential(prompt, saved.is_key)
                });
                let response = if automatic {
                    credential.take().map(|saved| saved.secret)
                } else {
                    next_id += 1;
                    let id = next_id;
                    let (sender, receiver) = mpsc::channel();
                    *pending.lock().unwrap() = Some(Pending {
                        id,
                        prompt: json!({"id":id,"prompt":prompt,"echo":hint == "confirm","informational":hint == "none"}),
                        response: sender,
                    });
                    changed();
                    let start = Instant::now();
                    let response = loop {
                        if stopped.load(Ordering::Acquire) || start.elapsed() >= AUTH_TIMEOUT {
                            break None;
                        }
                        match receiver.recv_timeout(Duration::from_millis(100)) {
                            Ok(response) => break Some(response),
                            Err(mpsc::RecvTimeoutError::Timeout) => continue,
                            Err(_) => break None,
                        }
                    };
                    let mut current = pending.lock().unwrap();
                    if current.as_ref().is_some_and(|p| p.id == id) {
                        *current = None;
                    }
                    drop(current);
                    changed();
                    response
                };
                // The response travels only through this private per-session channel.
                // It is never sent to terminal input, events, history, or profile storage.
                let _ = writeln!(stream, "{}", json!({"response":response}));
            }
        });
        Ok(auth)
    }

    pub fn prompt(&self) -> Option<Value> {
        self.pending
            .lock()
            .unwrap()
            .as_ref()
            .map(|p| p.prompt.clone())
    }

    pub fn respond(&self, id: u64, response: &str) -> Result<(), String> {
        if response.len() > 8192 || response.contains(['\r', '\n', '\0']) {
            return Err("认证响应必须为单行，且不能超过 8192 字节".into());
        }
        let mut pending = self.pending.lock().map_err(|_| "认证状态不可用")?;
        if pending.as_ref().is_none_or(|p| p.id != id) {
            return Err("认证提示已失效，请等待新的提示".into());
        }
        pending
            .take()
            .unwrap()
            .response
            .send(response.to_owned())
            .map_err(|_| "认证请求已结束".into())
    }
}

impl Drop for SshAuth {
    fn drop(&mut self) {
        self.stopped.store(true, Ordering::Release);
        self.pending.lock().unwrap().take();
    }
}

fn matches_credential(prompt: &str, is_key: bool) -> bool {
    let prompt = prompt.trim().to_ascii_lowercase();
    // OpenSSH prefixes keyboard-interactive prompts with (user@host).
    let prompt = if prompt.starts_with('(') {
        prompt
            .split_once(") ")
            .map_or(prompt.as_str(), |(_, text)| text.trim())
    } else {
        prompt.as_str()
    };
    if is_key {
        return prompt.starts_with("enter passphrase for key ") && prompt.ends_with(':');
    }
    if [
        "one-time",
        "one time",
        "otp",
        "verification",
        "token",
        "code",
        "duo",
        "2fa",
        "mfa",
    ]
    .iter()
    .any(|word| prompt.contains(word))
    {
        return false;
    }
    prompt == "password:"
        || prompt.ends_with("'s password:")
        || (prompt.starts_with("password for ") && prompt.ends_with(':'))
}

fn read_message(stream: &TcpStream) -> Result<Value, String> {
    let mut line = String::new();
    BufReader::new(stream.take(MAX_MESSAGE))
        .read_line(&mut line)
        .map_err(|e| e.to_string())?;
    if !line.ends_with('\n') {
        return Err("Invalid authentication message".into());
    }
    serde_json::from_str(&line).map_err(|e| e.to_string())
}

fn request_response(
    address: &str,
    token: &str,
    prompt: &str,
    hint: &str,
) -> Result<String, String> {
    let address: std::net::SocketAddr = address
        .parse()
        .map_err(|_| "Invalid authentication address")?;
    if !address.ip().is_loopback() {
        return Err("Authentication requires loopback".into());
    }
    let mut stream =
        TcpStream::connect_timeout(&address, Duration::from_secs(2)).map_err(|e| e.to_string())?;
    stream
        .set_read_timeout(Some(AUTH_TIMEOUT + Duration::from_secs(2)))
        .map_err(|e| e.to_string())?;
    stream
        .set_write_timeout(Some(Duration::from_secs(2)))
        .map_err(|e| e.to_string())?;
    let request = json!({"token":token,"prompt":prompt,"hint":hint}).to_string();
    if request.len() as u64 >= MAX_MESSAGE {
        return Err("Authentication prompt is too long".into());
    }
    writeln!(stream, "{request}").map_err(|e| e.to_string())?;
    read_message(&stream)?["response"]
        .as_str()
        .map(str::to_owned)
        .ok_or_else(|| "Authentication cancelled".into())
}

/// OpenSSH invokes this executable through SSH_ASKPASS, before Tauri starts.
pub fn run_askpass() -> i32 {
    let result = (|| {
        let address =
            std::env::var("SHELLINK_ASKPASS_ADDR").map_err(|_| "Missing authentication address")?;
        let token =
            std::env::var("SHELLINK_ASKPASS_TOKEN").map_err(|_| "Missing authentication token")?;
        let prompt = std::env::args().nth(1).unwrap_or_default();
        let hint = std::env::var("SSH_ASKPASS_PROMPT").unwrap_or_default();
        let response = request_response(&address, &token, &prompt, &hint)?;
        writeln!(std::io::stdout().lock(), "{response}").map_err(|e| e.to_string())
    })();
    if result.is_ok() { 0 } else { 1 }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn saved_credentials_never_answer_mfa_or_unknown_prompts() {
        assert!(matches_credential("user@host's password: ", false));
        assert!(matches_credential("Password:", false));
        assert!(matches_credential("(user@host) Password: ", false));
        assert!(matches_credential(
            "Enter passphrase for key '/tmp/id':",
            true
        ));
        for prompt in [
            "One-time password:",
            "OTP password:",
            "Password or verification code:",
            "Passcode or option (1-3):",
            "Verification code:",
            "(user@bastion.example.com) Please Input Mfa Code (SMS): ",
        ] {
            assert!(!matches_credential(prompt, false), "{prompt}");
        }
        assert!(!matches_credential("Password:", true));
        assert!(!matches_credential(
            "Enter passphrase for key '/tmp/id':",
            false
        ));
    }

    #[test]
    fn password_then_multiple_mfa_challenges_use_separate_responses() {
        let (sender, receiver) = mpsc::channel();
        let auth = SshAuth::start(
            Some(Credential {
                secret: "saved-password".into(),
                is_key: false,
            }),
            move || {
                let _ = sender.send(());
            },
        )
        .unwrap();
        assert_eq!(
            request_response(&auth.address, &auth.token, "Password:", "").unwrap(),
            "saved-password"
        );
        for (prompt, response) in [
            ("Verification code:", "123456"),
            ("Password:", "corrected-password"),
            ("Duo: select an option", "1"),
        ] {
            let address = auth.address.clone();
            let token = auth.token.clone();
            let client = thread::spawn(move || request_response(&address, &token, prompt, ""));
            let deadline = Instant::now() + Duration::from_secs(3);
            let pending = loop {
                receiver
                    .recv_timeout(deadline.saturating_duration_since(Instant::now()))
                    .expect("Missing authentication prompt");
                if let Some(pending) = auth.prompt() {
                    break pending;
                }
            };
            assert_eq!(pending["prompt"], prompt);
            let id = pending["id"].as_u64().unwrap();
            assert!(auth.respond(id + 1, "stale").is_err());
            assert!(auth.respond(id, "bad\nresponse").is_err());
            auth.respond(id, response).unwrap();
            assert!(auth.respond(id, "duplicate").is_err());
            assert_eq!(client.join().unwrap().unwrap(), response);
        }
    }

    #[test]
    fn cancellation_and_invalid_tokens_do_not_return_secrets() {
        let (sender, receiver) = mpsc::channel();
        let auth = SshAuth::start(None, move || {
            let _ = sender.send(());
        })
        .unwrap();
        assert!(request_response(&auth.address, "incorrect-token", "Password:", "").is_err());
        let address = auth.address.clone();
        let token = auth.token.clone();
        let client = thread::spawn(move || {
            request_response(&address, &token, "Approve sign-in on your phone", "none")
        });
        receiver.recv_timeout(Duration::from_secs(3)).unwrap();
        assert_eq!(auth.prompt().unwrap()["informational"], true);
        drop(auth);
        assert!(client.join().unwrap().is_err());
    }
}
