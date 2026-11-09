use super::*;
use std::{fs, net::SocketAddr, path::Path};

const DISCOVERY_FILE: &str = "mcp-runtime.json";

pub(super) fn write_discovery(
    home: &Path,
    address: SocketAddr,
) -> Result<(PathBuf, String), String> {
    let path = home.join(DISCOVERY_FILE);
    let contents = json!({"address":address.to_string(),"pid":std::process::id()}).to_string();
    let temporary = home.join(format!(
        ".mcp-runtime-{}-{}.tmp",
        std::process::id(),
        crate::data::session_now()
    ));
    let result = (|| {
        let mut options = fs::OpenOptions::new();
        options.write(true).create_new(true);
        #[cfg(unix)]
        {
            use std::os::unix::fs::OpenOptionsExt;
            options.mode(0o600);
        }
        let mut file = options.open(&temporary).map_err(|e| e.to_string())?;
        file.write_all(contents.as_bytes())
            .map_err(|e| e.to_string())?;
        file.sync_all().map_err(|e| e.to_string())?;
        drop(file);
        fs::rename(&temporary, &path).map_err(|e| e.to_string())?;
        Ok((path, contents))
    })();
    if result.is_err() {
        let _ = fs::remove_file(&temporary);
    }
    result
}

fn discovery_address(home: &Path) -> Result<SocketAddr, String> {
    let file = fs::File::open(home.join(DISCOVERY_FILE))
        .map_err(|_| "Desktop MCP is unavailable. Start the shellink Desktop first.".to_owned())?;
    let mut text = String::new();
    file.take(4097)
        .read_to_string(&mut text)
        .map_err(|e| e.to_string())?;
    if text.len() > 4096 {
        return Err("invalid Desktop MCP discovery file".into());
    }
    let runtime: Value = serde_json::from_str(&text).map_err(|e| e.to_string())?;
    let address = runtime["address"]
        .as_str()
        .ok_or("missing Desktop MCP address")?
        .parse::<SocketAddr>()
        .map_err(|e| e.to_string())?;
    if !address.ip().is_loopback() || address.port() == 0 {
        return Err("Desktop MCP bridge must use a loopback address".into());
    }
    Ok(address)
}

pub fn run_stdio() {
    let db = match DataStore::open() {
        Ok(db) => db,
        Err(error) => {
            let _ = writeln!(io::stderr(), "shellink MCP stdio unavailable: {error}");
            return;
        }
    };
    let mut input = io::stdin().lock();
    let mut stdout = io::BufWriter::new(io::stdout().lock());
    let mut version: Option<String> = None;
    loop {
        let mut line = Vec::new();
        match input
            .by_ref()
            .take(MAX_BODY_BYTES as u64 + 1)
            .read_until(b'\n', &mut line)
        {
            Ok(0) => break,
            Ok(_) if line.len() > MAX_BODY_BYTES => {
                let _ = writeln!(io::stderr(), "shellink MCP stdio message exceeds 4 MiB");
                break;
            }
            Ok(_) => {}
            Err(error) => {
                let _ = writeln!(io::stderr(), "shellink MCP stdio read failed: {error}");
                break;
            }
        }
        if line.iter().all(u8::is_ascii_whitespace) {
            continue;
        }
        let request: Value = match serde_json::from_slice(&line) {
            Ok(request) => request,
            Err(error) => {
                let response = json!({"jsonrpc":"2.0","id":null,"error":{"code":-32700,"message":format!("parse error: {error}")}});
                let _ = writeln!(stdout, "{response}");
                let _ = stdout.flush();
                continue;
            }
        };
        let notification = request
            .as_object()
            .is_some_and(|object| object.contains_key("method") && !object.contains_key("id"));
        match proxy_request(&db, &request, version.as_deref()) {
            Ok(Some(response)) if !notification => {
                if request["method"] == "initialize" {
                    version = response["result"]["protocolVersion"]
                        .as_str()
                        .map(str::to_owned);
                }
                let _ = writeln!(stdout, "{response}");
                if stdout.flush().is_err() {
                    break;
                }
            }
            Ok(_) => {}
            Err(error) if !notification => {
                let id = request.get("id").cloned().unwrap_or(Value::Null);
                let response =
                    json!({"jsonrpc":"2.0","id":id,"error":{"code":-32000,"message":error}});
                let _ = writeln!(stdout, "{response}");
                if stdout.flush().is_err() {
                    break;
                }
            }
            Err(error) => {
                let _ = writeln!(io::stderr(), "shellink MCP request failed: {error}");
            }
        }
    }
}

fn proxy_request(
    db: &DataStore,
    request: &Value,
    version: Option<&str>,
) -> Result<Option<Value>, String> {
    let address = discovery_address(db.data_home())?;
    for attempt in 0..2 {
        // Refresh for every message and once after a 401; never replay an input after an I/O failure.
        let info = db.mcp_info()?;
        let token = info["token"].as_str().ok_or("missing MCP token")?;
        let (status, response) = http_request(address, token, request, version)?;
        if status == 401 && attempt == 0 {
            continue;
        }
        if status >= 400 {
            return Err(response
                .as_ref()
                .and_then(|v| v["error"].as_str())
                .unwrap_or("Desktop MCP request failed")
                .to_owned());
        }
        return Ok(response);
    }
    Err("Desktop MCP authentication failed".into())
}

fn http_request(
    address: SocketAddr,
    token: &str,
    request: &Value,
    version: Option<&str>,
) -> Result<(u16, Option<Value>), String> {
    let mut stream = TcpStream::connect_timeout(&address, Duration::from_secs(3))
        .map_err(|error| format!("Desktop MCP bridge is unavailable: {error}"))?;
    stream
        .set_read_timeout(Some(Duration::from_secs(35)))
        .map_err(|e| e.to_string())?;
    stream
        .set_write_timeout(Some(Duration::from_secs(10)))
        .map_err(|e| e.to_string())?;
    let body = serde_json::to_vec(request).map_err(|e| e.to_string())?;
    if body.len() > MAX_BODY_BYTES {
        return Err("MCP request is too large".into());
    }
    let protocol = version
        .map(|version| format!("MCP-Protocol-Version: {version}\r\n"))
        .unwrap_or_default();
    write!(stream, "POST /shellink/mcp HTTP/1.1\r\nHost: {address}\r\nAuthorization: Bearer {token}\r\nContent-Type: application/json\r\nAccept: application/json, text/event-stream\r\n{protocol}Content-Length: {}\r\nConnection: close\r\n\r\n", body.len()).map_err(|e| e.to_string())?;
    stream.write_all(&body).map_err(|e| e.to_string())?;
    stream.flush().map_err(|e| e.to_string())?;
    let mut response = Vec::new();
    stream
        .take(transport::MAX_RESPONSE_BYTES as u64 + 1)
        .read_to_end(&mut response)
        .map_err(|e| e.to_string())?;
    if response.len() > transport::MAX_RESPONSE_BYTES {
        return Err("Desktop MCP response is too large".into());
    }
    let end = response
        .windows(4)
        .position(|window| window == b"\r\n\r\n")
        .ok_or("invalid Desktop MCP HTTP response")?;
    if end > MAX_HEADER_BYTES {
        return Err("Desktop MCP response headers are too large".into());
    }
    let header = std::str::from_utf8(&response[..end]).map_err(|e| e.to_string())?;
    let status = header
        .lines()
        .next()
        .and_then(|line| line.split_whitespace().nth(1))
        .and_then(|v| v.parse::<u16>().ok())
        .ok_or("invalid Desktop MCP HTTP status")?;
    let payload = &response[end + 4..];
    if status == 202 && payload.is_empty() {
        return Ok((status, None));
    }
    let value = serde_json::from_slice(payload)
        .map_err(|e| format!("invalid Desktop MCP response: {e}"))?;
    Ok((status, Some(value)))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn proxy_refreshes_token_and_retries_only_rejected_authentication() {
        let home = std::env::temp_dir().join(format!(
            "shellink-rotation-{}-{}",
            std::process::id(),
            crate::data::session_now()
        ));
        fs::create_dir_all(&home).unwrap();
        let db = Arc::new(DataStore::test_store(home.clone()));
        let listener = TcpListener::bind("127.0.0.1:0").unwrap();
        write_discovery(&home, listener.local_addr().unwrap()).unwrap();
        let first_token = db.mcp_info().unwrap()["token"].as_str().unwrap().to_owned();
        let server_db = db.clone();
        let server = thread::spawn(move || {
            for attempt in 0..3 {
                let (mut stream, _) = listener.accept().unwrap();
                let request = transport::read_request(&mut stream).unwrap();
                let current = server_db.mcp_info().unwrap()["token"]
                    .as_str()
                    .unwrap()
                    .to_owned();
                assert_eq!(
                    request.headers["authorization"],
                    format!("Bearer {current}")
                );
                if attempt == 0 {
                    assert_eq!(current, first_token);
                }
                if attempt == 1 {
                    server_db
                        .mcp_config(&json!({"regenerateToken":true}))
                        .unwrap();
                    transport::respond_error(&mut stream, 401, "unauthorized");
                } else {
                    let request: Value = serde_json::from_slice(&request.body).unwrap();
                    let body = json!({"jsonrpc":"2.0","id":request["id"],"result":{}}).to_string();
                    write!(
                        stream,
                        "HTTP/1.1 200 OK\r\nContent-Length: {}\r\n\r\n{body}",
                        body.len()
                    )
                    .unwrap();
                }
            }
        });
        assert_eq!(
            proxy_request(&db, &json!({"id":1,"method":"ping"}), None)
                .unwrap()
                .unwrap()["id"],
            1
        );
        db.mcp_config(&json!({"regenerateToken":true})).unwrap();
        assert_eq!(
            proxy_request(&db, &json!({"id":2,"method":"ping"}), None)
                .unwrap()
                .unwrap()["id"],
            2
        );
        server.join().unwrap();
        fs::remove_dir_all(home).unwrap();
    }

    #[test]
    fn discovery_rejects_non_loopback_and_is_private() {
        let home = std::env::temp_dir().join(format!(
            "shellink-discovery-{}-{}",
            std::process::id(),
            crate::data::session_now()
        ));
        fs::create_dir_all(&home).unwrap();
        let address = "127.0.0.1:12345".parse().unwrap();
        let (path, _) = write_discovery(&home, address).unwrap();
        assert_eq!(discovery_address(&home).unwrap(), address);
        #[cfg(unix)]
        {
            use std::os::unix::fs::PermissionsExt;
            assert_eq!(
                fs::metadata(&path).unwrap().permissions().mode() & 0o777,
                0o600
            );
        }
        fs::write(&path, r#"{"address":"192.0.2.1:7070"}"#).unwrap();
        assert!(discovery_address(&home).is_err());
        fs::remove_dir_all(home).unwrap();
    }

    #[test]
    fn proxy_preserves_json_rpc_and_empty_notifications() {
        let listener = TcpListener::bind("127.0.0.1:0").unwrap();
        let address = listener.local_addr().unwrap();
        let server = thread::spawn(move || {
            for notification in [false, true] {
                let (mut stream, _) = listener.accept().unwrap();
                let request = transport::read_request(&mut stream).unwrap();
                assert_eq!(request.headers["authorization"], "Bearer test-token");
                assert_eq!(request.headers["mcp-protocol-version"], "2025-06-18");
                if notification {
                    write!(stream, "HTTP/1.1 202 Accepted\r\nContent-Length: 0\r\n\r\n").unwrap();
                } else {
                    let body = r#"{"jsonrpc":"2.0","id":7,"result":{}}"#;
                    write!(
                        stream,
                        "HTTP/1.1 200 OK\r\nContent-Length: {}\r\n\r\n{body}",
                        body.len()
                    )
                    .unwrap();
                }
            }
        });
        assert_eq!(
            http_request(
                address,
                "test-token",
                &json!({"id":7,"method":"ping"}),
                Some("2025-06-18")
            )
            .unwrap()
            .1
            .unwrap()["id"],
            7
        );
        assert!(
            http_request(
                address,
                "test-token",
                &json!({"method":"notifications/initialized"}),
                Some("2025-06-18")
            )
            .unwrap()
            .1
            .is_none()
        );
        server.join().unwrap();
    }
}
