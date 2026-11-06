use super::*;
use std::net::Shutdown;
use std::sync::atomic::AtomicUsize;
use std::time::Instant;

const MAX_CONNECTIONS: usize = 32;
pub(super) const MAX_RESPONSE_BYTES: usize = 8 * 1024 * 1024;

pub(super) struct Listener {
    pub stop: Arc<AtomicBool>,
    pub running: Arc<AtomicBool>,
    connections: Arc<Mutex<HashMap<usize, TcpStream>>>,
    join: thread::JoinHandle<()>,
}

impl Listener {
    pub fn stop(self) {
        self.stop.store(true, Ordering::Release);
        for stream in self.connections.lock().unwrap().values() {
            let _ = stream.shutdown(Shutdown::Both);
        }
        let _ = self.join.join();
    }
}

pub(super) fn spawn_listener(
    listener: TcpListener,
    app: AppHandle,
    internal: bool,
) -> io::Result<Listener> {
    listener.set_nonblocking(true)?;
    let stop = Arc::new(AtomicBool::new(false));
    let running = Arc::new(AtomicBool::new(true));
    let connections = Arc::new(Mutex::new(HashMap::<usize, TcpStream>::new()));
    let thread_stop = stop.clone();
    let thread_running = running.clone();
    let thread_connections = connections.clone();
    let next = AtomicUsize::new(0);
    let join = thread::spawn(move || {
        let mut workers: Vec<thread::JoinHandle<()>> = Vec::new();
        while !thread_stop.load(Ordering::Acquire) {
            let mut i = 0;
            while i < workers.len() {
                if workers[i].is_finished() {
                    let _ = workers.swap_remove(i).join();
                } else {
                    i += 1;
                }
            }
            match listener.accept() {
                Ok((mut stream, _)) => {
                    if workers.len() >= MAX_CONNECTIONS {
                        let _ = stream.set_write_timeout(Some(Duration::from_millis(100)));
                        respond_error(&mut stream, 503, "too many concurrent requests");
                        continue;
                    }
                    let id = next.fetch_add(1, Ordering::Relaxed);
                    if let Ok(copy) = stream.try_clone() {
                        thread_connections.lock().unwrap().insert(id, copy);
                    }
                    let app = app.clone();
                    let stop = thread_stop.clone();
                    let connections = thread_connections.clone();
                    workers.push(thread::spawn(move || {
                        if !stop.load(Ordering::Acquire) {
                            handle(stream, app, internal, &stop);
                        }
                        connections.lock().unwrap().remove(&id);
                    }));
                }
                Err(error) if error.kind() == io::ErrorKind::WouldBlock => {
                    thread::sleep(Duration::from_millis(25))
                }
                Err(_) => break,
            }
        }
        for stream in thread_connections.lock().unwrap().values() {
            let _ = stream.shutdown(Shutdown::Both);
        }
        for worker in workers {
            let _ = worker.join();
        }
        thread_running.store(false, Ordering::Release);
    });
    Ok(Listener {
        stop,
        running,
        connections,
        join,
    })
}

fn handle(mut stream: TcpStream, app: AppHandle, internal: bool, stop: &AtomicBool) {
    let _ = stream.set_read_timeout(Some(Duration::from_secs(10)));
    let _ = stream.set_write_timeout(Some(Duration::from_secs(10)));
    let request = match read_request(&mut stream) {
        Ok(request) => request,
        Err((status, message)) => {
            respond_error(&mut stream, status, &message);
            return;
        }
    };
    let port = stream
        .local_addr()
        .map(|addr| addr.port())
        .unwrap_or_else(|_| http_port());
    if let Err((status, message)) = validate_request(&request, internal, &bind_host(), port) {
        respond_error(&mut stream, status, &message);
        return;
    }
    let expected = match app.state::<DataStore>().mcp_info() {
        Ok(info) => info["token"].as_str().unwrap_or_default().to_owned(),
        Err(error) => {
            respond_error(&mut stream, 503, &error);
            return;
        }
    };
    if expected.is_empty()
        || request.headers.get("authorization") != Some(&format!("Bearer {expected}"))
    {
        respond_error(&mut stream, 401, "unauthorized");
        return;
    }
    let request = match serde_json::from_slice::<Value>(&request.body) {
        Ok(value) => value,
        Err(_) => {
            respond_json_rpc_error(&mut stream, 400, Value::Null, -32700, "parse error");
            return;
        }
    };
    match process_request(&app, request, stop, internal) {
        Ok(Some(response)) => respond_json(&mut stream, 200, response),
        Ok(None) => {
            let _ = write!(
                stream,
                "HTTP/1.1 202 Accepted\r\nContent-Length: 0\r\nConnection: close\r\n\r\n"
            );
        }
        Err((id, code, message)) => respond_json_rpc_error(&mut stream, 200, id, code, &message),
    }
}

#[derive(Debug)]
pub(super) struct HttpRequest {
    pub method: String,
    pub path: String,
    pub headers: HashMap<String, String>,
    pub body: Vec<u8>,
}

fn validate_request(
    request: &HttpRequest,
    internal: bool,
    host: &str,
    port: u16,
) -> Result<(), (u16, String)> {
    if request.path != "/shellink/mcp" {
        return Err((404, "not found".into()));
    }
    if let Some(origin) = request.headers.get("origin") {
        if !allowed_origin(origin, host, port, internal) {
            return Err((403, "origin is not allowed".into()));
        }
    }
    if request.method != "POST" {
        return Err((405, "method not allowed".into()));
    }
    let content_type = request
        .headers
        .get("content-type")
        .and_then(|value| value.split(';').next())
        .unwrap_or_default()
        .trim();
    if !content_type.eq_ignore_ascii_case("application/json") {
        return Err((415, "Content-Type must be application/json".into()));
    }
    if request.path == "/shellink/mcp" {
        if let Some(version) = request.headers.get("mcp-protocol-version") {
            if !SUPPORTED_VERSIONS.contains(&version.as_str())
                || (!internal && version == "2024-11-05")
            {
                return Err((400, "unsupported MCP-Protocol-Version".into()));
            }
        }
        let accept = request
            .headers
            .get("accept")
            .map(String::as_str)
            .unwrap_or_default();
        let accepts_json = accept.split(',').any(|part| {
            matches!(
                part.trim().split(';').next(),
                Some("application/json" | "*/*")
            )
        });
        let accepts_sse = accept.split(',').any(|part| {
            matches!(
                part.trim().split(';').next(),
                Some("text/event-stream" | "*/*")
            )
        });
        if !accepts_json || !accepts_sse {
            return Err((
                406,
                "Accept must include application/json and text/event-stream".into(),
            ));
        }
    }
    Ok(())
}

fn allowed_origin(origin: &str, host: &str, port: u16, internal: bool) -> bool {
    let Ok(origin) = tauri::Url::parse(origin) else {
        return false;
    };
    if origin.scheme() != "http"
        || origin.port_or_known_default() != Some(port)
        || origin.path() != "/"
        || origin.query().is_some()
        || origin.fragment().is_some()
        || !origin.username().is_empty()
        || origin.password().is_some()
    {
        return false;
    }
    let Some(origin_host) = origin.host_str() else {
        return false;
    };
    let origin_host = origin_host.trim_matches(['[', ']']);
    if internal || matches!(host, "127.0.0.1" | "::1" | "localhost" | "0.0.0.0" | "::") {
        return matches!(origin_host, "127.0.0.1" | "::1" | "localhost");
    }
    origin_host.eq_ignore_ascii_case(host)
}

fn bounded_line(reader: &mut impl BufRead, budget: usize) -> Result<String, (u16, String)> {
    let mut bytes = Vec::new();
    reader
        .take(budget as u64 + 1)
        .read_until(b'\n', &mut bytes)
        .map_err(|error| (400, error.to_string()))?;
    if bytes.len() > budget {
        return Err((431, "HTTP headers too large".into()));
    }
    if !bytes.ends_with(b"\n") {
        return Err((400, "incomplete HTTP headers".into()));
    }
    String::from_utf8(bytes).map_err(|_| (400, "invalid HTTP headers".into()))
}

pub(super) fn read_request(stream: &mut TcpStream) -> Result<HttpRequest, (u16, String)> {
    let deadline = Instant::now() + Duration::from_secs(10);
    struct DeadlineReader<'a> {
        stream: &'a mut TcpStream,
        deadline: Instant,
    }
    impl Read for DeadlineReader<'_> {
        fn read(&mut self, bytes: &mut [u8]) -> io::Result<usize> {
            let remaining = self.deadline.saturating_duration_since(Instant::now());
            if remaining.is_zero() {
                return Err(io::Error::new(io::ErrorKind::TimedOut, "request timed out"));
            }
            self.stream.set_read_timeout(Some(remaining))?;
            self.stream.read(bytes)
        }
    }
    let mut reader = BufReader::new(DeadlineReader { stream, deadline });
    let first = bounded_line(&mut reader, MAX_HEADER_BYTES)?;
    let parts: Vec<_> = first.split_whitespace().collect();
    if parts.len() != 3 || !matches!(parts[2], "HTTP/1.0" | "HTTP/1.1") {
        return Err((400, "invalid HTTP request line".into()));
    }
    let (method, path) = (parts[0].to_owned(), parts[1].to_owned());
    let mut budget = MAX_HEADER_BYTES - first.len();
    let mut headers = HashMap::new();
    loop {
        let line = bounded_line(&mut reader, budget)?;
        budget -= line.len();
        if line == "\r\n" || line == "\n" {
            break;
        }
        if Instant::now() > deadline {
            return Err((408, "request timed out".into()));
        }
        let Some((name, value)) = line.trim_end_matches(['\r', '\n']).split_once(':') else {
            return Err((400, "invalid HTTP header".into()));
        };
        if name.is_empty()
            || !name
                .bytes()
                .all(|b| b.is_ascii_alphanumeric() || b"!#$%&'*+-.^_`|~".contains(&b))
            || value.bytes().any(|b| b < 32 && b != b'\t')
        {
            return Err((400, "invalid HTTP header".into()));
        }
        if headers
            .insert(name.to_ascii_lowercase(), value.trim().to_owned())
            .is_some()
        {
            return Err((400, "duplicate HTTP header".into()));
        }
    }
    let mut body = Vec::new();
    if let Some(encoding) = headers.get("transfer-encoding") {
        if !encoding.eq_ignore_ascii_case("chunked") || headers.contains_key("content-length") {
            return Err((400, "invalid Transfer-Encoding".into()));
        }
        loop {
            let size = bounded_line(&mut reader, 128)?;
            let size = usize::from_str_radix(size.trim().split(';').next().unwrap_or_default(), 16)
                .map_err(|_| (400, "invalid chunk size".into()))?;
            if size > MAX_BODY_BYTES - body.len() {
                return Err((413, "request body too large".into()));
            }
            if size == 0 {
                loop {
                    let trailer = bounded_line(&mut reader, budget)?;
                    budget -= trailer.len();
                    if trailer == "\r\n" || trailer == "\n" {
                        break;
                    }
                }
                break;
            }
            let start = body.len();
            body.resize(start + size, 0);
            reader
                .read_exact(&mut body[start..])
                .map_err(|e| (400, e.to_string()))?;
            let mut ending = [0; 2];
            reader
                .read_exact(&mut ending)
                .map_err(|e| (400, e.to_string()))?;
            if ending != *b"\r\n" {
                return Err((400, "invalid chunk ending".into()));
            }
            if Instant::now() > deadline {
                return Err((408, "request timed out".into()));
            }
        }
    } else if method == "POST" || headers.contains_key("content-length") {
        let size = parse_content_length(headers.get("content-length").map(String::as_str))?;
        body.resize(size, 0);
        reader
            .read_exact(&mut body)
            .map_err(|error| (400, format!("incomplete request body: {error}")))?;
    }
    Ok(HttpRequest {
        method,
        path,
        headers,
        body,
    })
}

pub(super) fn parse_content_length(value: Option<&str>) -> Result<usize, (u16, String)> {
    let value = value.ok_or_else(|| (411, "Content-Length is required".into()))?;
    if value.is_empty() || !value.bytes().all(|b| b.is_ascii_digit()) {
        return Err((400, "invalid Content-Length".into()));
    }
    let size = value
        .parse::<usize>()
        .map_err(|_| (400, "invalid Content-Length".into()))?;
    if size > MAX_BODY_BYTES {
        return Err((413, "request body too large".into()));
    }
    Ok(size)
}

pub(super) fn respond_error(stream: &mut TcpStream, status: u16, message: &str) {
    respond_json(stream, status, json!({"error":message}));
}

fn respond_json_rpc_error(
    stream: &mut TcpStream,
    status: u16,
    id: Value,
    code: i64,
    message: &str,
) {
    respond_json(
        stream,
        status,
        json!({"jsonrpc":"2.0","id":id,"error":{"code":code,"message":message}}),
    );
}

fn respond_json(stream: &mut TcpStream, status: u16, value: Value) {
    let body = value.to_string();
    let reason = match status {
        200 => "OK",
        202 => "Accepted",
        400 => "Bad Request",
        401 => "Unauthorized",
        403 => "Forbidden",
        404 => "Not Found",
        405 => "Method Not Allowed",
        406 => "Not Acceptable",
        408 => "Request Timeout",
        411 => "Length Required",
        413 => "Payload Too Large",
        415 => "Unsupported Media Type",
        431 => "Request Header Fields Too Large",
        503 => "Service Unavailable",
        _ => "Error",
    };
    let allow = if status == 405 { "Allow: POST\r\n" } else { "" };
    let _ = write!(
        stream,
        "HTTP/1.1 {status} {reason}\r\nContent-Type: application/json\r\n{allow}Content-Length: {}\r\nConnection: close\r\n\r\n{body}",
        body.len()
    );
}

#[cfg(test)]
mod tests {
    use super::*;

    fn parse(raw: Vec<u8>) -> Result<HttpRequest, (u16, String)> {
        let listener = TcpListener::bind("127.0.0.1:0").unwrap();
        let addr = listener.local_addr().unwrap();
        let client = thread::spawn(move || {
            let mut stream = TcpStream::connect(addr).unwrap();
            let _ = stream.write_all(&raw);
        });
        let (mut stream, _) = listener.accept().unwrap();
        let result = read_request(&mut stream);
        client.join().unwrap();
        result
    }

    #[test]
    fn bounds_headers_and_rejects_ambiguous_framing() {
        let oversized = format!(
            "POST /shellink/mcp HTTP/1.1\r\nX: {}",
            "a".repeat(MAX_HEADER_BYTES)
        );
        assert_eq!(parse(oversized.into_bytes()).unwrap_err().0, 431);
        assert_eq!(
            parse(b"POST / HTTP/1.1\r\nContent-Length: 0\r\nContent-Length: 1\r\n\r\n".to_vec())
                .unwrap_err()
                .0,
            400
        );
        assert_eq!(
            parse(
                b"POST / HTTP/1.1\r\nContent-Length: 0\r\nTransfer-Encoding: chunked\r\n\r\n"
                    .to_vec()
            )
            .unwrap_err()
            .0,
            400
        );
    }

    #[test]
    fn reads_chunked_json_and_get_without_length() {
        let request = parse(b"POST /shellink/mcp HTTP/1.1\r\nTransfer-Encoding: chunked\r\n\r\n3\r\n{\"x\r\n4\r\n\":1}\r\n0\r\n\r\n".to_vec()).unwrap();
        assert_eq!(request.body, b"{\"x\":1}");
        let get = parse(b"GET /shellink/mcp HTTP/1.1\r\n\r\n".to_vec()).unwrap();
        assert_eq!(
            validate_request(&get, false, "127.0.0.1", 7070)
                .unwrap_err()
                .0,
            405
        );
    }

    #[test]
    fn validates_origins_protocol_and_media_types() {
        let mut request = HttpRequest {
            method: "POST".into(),
            path: "/shellink/mcp".into(),
            headers: HashMap::from([
                (
                    "content-type".into(),
                    "application/json; charset=utf-8".into(),
                ),
                (
                    "accept".into(),
                    "application/json, text/event-stream".into(),
                ),
            ]),
            body: vec![],
        };
        assert!(validate_request(&request, false, "127.0.0.1", 7070).is_ok());
        for origin in [
            "null",
            "http://evil.test:7070",
            "http://localhost:9999",
            "http://localhost:7070/path",
        ] {
            request.headers.insert("origin".into(), origin.into());
            assert_eq!(
                validate_request(&request, false, "127.0.0.1", 7070)
                    .unwrap_err()
                    .0,
                403
            );
        }
        request
            .headers
            .insert("origin".into(), "http://localhost:7070".into());
        request
            .headers
            .insert("mcp-protocol-version".into(), "2025-06-18".into());
        assert!(validate_request(&request, false, "127.0.0.1", 7070).is_ok());
        request
            .headers
            .insert("mcp-protocol-version".into(), "garbage".into());
        assert_eq!(
            validate_request(&request, false, "127.0.0.1", 7070)
                .unwrap_err()
                .0,
            400
        );
    }
}
