//! Shared terminal-text helpers for history storage, MCP tools, and events.

/// ANSI removal for a log stream. Parser state survives PTY read boundaries;
/// no screen grid, cursor emulation, or terminal rendering is performed.
///
/// Besides stripping escapes, C0 bytes with no textual meaning (BEL, BS, NUL)
/// are dropped. CR/LF/TAB stay verbatim: they are part of the raw stream and
/// this helper deliberately does no screen emulation.
#[derive(Default)]
pub struct AnsiStripper {
    state: State,
}

#[derive(Default)]
enum State {
    #[default]
    Ground,
    Escape,
    Intermediate,
    Csi,
    Osc,
    OscEscape,
    String,
    StringEscape,
    /// ESC O — SS3 / application cursor keys. Consumes exactly one more byte.
    Ss3,
}

impl AnsiStripper {
    pub fn feed(&mut self, text: &str) -> String {
        let mut out = String::with_capacity(text.len());
        for ch in text.chars() {
            let state = std::mem::replace(&mut self.state, State::Ground);
            self.state = match state {
                State::Ground => match ch {
                    '\x1b' => State::Escape,
                    '\u{9b}' => State::Csi,
                    '\u{9d}' => State::Osc,
                    // CRLF and in-place redraws are meaningful in a log stream.
                    '\r' | '\n' | '\t' => {
                        out.push(ch);
                        State::Ground
                    }
                    // BEL, BS, NUL and friends carry no text in a log stream.
                    c if c.is_control() => State::Ground,
                    _ => {
                        out.push(ch);
                        State::Ground
                    }
                },
                State::Escape => match ch {
                    '[' => State::Csi,
                    ']' => State::Osc,
                    'O' => State::Ss3,
                    'P' | 'X' | '^' | '_' => State::String,
                    '\x1b' => State::Escape,
                    '\u{20}'..='\u{2f}' => State::Intermediate,
                    _ => State::Ground,
                },
                // SS3 sequences such as ESC O A (Up) leave a stray letter behind
                // when parsed as an ordinary escape; swallow the final byte.
                State::Ss3 => State::Ground,
                State::Intermediate => match ch {
                    '\x1b' => State::Escape,
                    '\u{20}'..='\u{2f}' => State::Intermediate,
                    _ => State::Ground,
                },
                State::Csi => match ch {
                    '\x1b' => State::Escape,
                    '\u{40}'..='\u{7e}' => State::Ground,
                    _ => State::Csi,
                },
                State::Osc => match ch {
                    '\x07' | '\u{9c}' => State::Ground,
                    '\x1b' => State::OscEscape,
                    _ => State::Osc,
                },
                State::OscEscape => match ch {
                    '\\' | '\x07' | '\u{9c}' => State::Ground,
                    '\x1b' => State::OscEscape,
                    _ => State::Osc,
                },
                State::String => match ch {
                    '\u{9c}' => State::Ground,
                    '\x1b' => State::StringEscape,
                    _ => State::String,
                },
                State::StringEscape => match ch {
                    '\\' | '\u{9c}' => State::Ground,
                    '\x1b' => State::StringEscape,
                    _ => State::String,
                },
            };
        }
        out
    }
}

#[cfg(test)]
pub fn strip_ansi(text: &str) -> String {
    AnsiStripper::default().feed(text)
}

/// A submitted input line: exact bytes sent to the PTY plus the readable
/// rendering stored as `data_plain`.
#[derive(Debug, PartialEq, Eq)]
pub struct InputLine {
    pub raw: String,
    pub display: String,
}

/// Accumulates xterm `onData` keystrokes into one history entry per submitted
/// command. Control keys become visible markers instead of producing empty,
/// single-character, or corrupted chunks. Parser state survives chunk
/// boundaries, so an escape sequence split across two calls is handled.
#[derive(Default)]
pub struct InputLineBuffer {
    raw: String,
    display: String,
    /// An escape sequence being collected, without the leading ESC byte.
    escape: Option<String>,
    crlf_pending: bool,
}

impl InputLineBuffer {
    /// Feed one raw keystroke chunk. Returns every command completed by a CR,
    /// LF or Ctrl-C in this chunk.
    pub fn feed(&mut self, text: &str) -> Vec<InputLine> {
        let mut finished = Vec::new();
        for ch in text.chars() {
            if self.crlf_pending {
                self.crlf_pending = false;
                if ch == '\n' {
                    self.raw.push(ch);
                    continue;
                }
            }
            self.raw.push(ch);

            if self.escape.is_some() {
                if let Some(seq) = self.escape.as_mut() {
                    seq.push(ch);
                }
                let (complete, marker) = {
                    let seq = self.escape.as_deref().unwrap_or_default();
                    let complete = if let Some(rest) = seq.strip_prefix('[') {
                        !rest.is_empty() && ('\u{40}'..='\u{7e}').contains(&ch)
                    } else if seq.starts_with('O') {
                        seq.len() == 2
                    } else {
                        true
                    };
                    (complete, escape_marker(seq))
                };
                if complete {
                    self.display.push_str(&marker);
                    self.escape = None;
                }
                continue;
            }

            match ch {
                '\x1b' => self.escape = Some(String::new()),
                '\r' | '\n' => {
                    self.crlf_pending = ch == '\r';
                    self.submit(&mut finished);
                }
                // Backspace/DEL edit the current line rather than becoming a chunk.
                '\x7f' | '\x08' => {
                    self.display.pop();
                }
                '\x03' => {
                    self.display.push_str("^C");
                    self.submit(&mut finished);
                }
                '\t' => self.display.push_str("⇥"),
                c if c.is_control() => self.display.push_str(&caret_marker(c)),
                _ => self.display.push(ch),
            }
        }
        finished
    }

    fn submit(&mut self, finished: &mut Vec<InputLine>) {
        let raw = std::mem::take(&mut self.raw);
        let display = std::mem::take(&mut self.display);
        if !display.trim().is_empty() {
            finished.push(InputLine { raw, display });
        }
    }
}

/// Make control characters visible in a whole API input. Newlines are kept so
/// multi-line scripts stay legible; CSI/SS3 cursor keys become arrow glyphs
/// instead of raw `^[[A`, and a lone Tab becomes `⇥` so a raw-key call is never
/// stored as an apparently empty entry.
pub fn readable_command(text: &str) -> String {
    let mut out = String::with_capacity(text.len());
    let mut escape: Option<String> = None;
    for ch in text.chars() {
        if escape.is_some() {
            if let Some(seq) = escape.as_mut() {
                seq.push(ch);
            }
            let (complete, marker) = {
                let seq = escape.as_deref().unwrap_or_default();
                let complete = if let Some(rest) = seq.strip_prefix('[') {
                    !rest.is_empty() && ('\u{40}'..='\u{7e}').contains(&ch)
                } else if seq.starts_with('O') {
                    seq.len() == 2
                } else {
                    true
                };
                (complete, escape_marker(seq))
            };
            if complete {
                out.push_str(&marker);
                escape = None;
            }
            continue;
        }
        match ch {
            '\x1b' => escape = Some(String::new()),
            '\n' => out.push(ch),
            '\t' => out.push('⇥'),
            '\r' => {}
            c if c.is_control() => out.push_str(&caret_marker(c)),
            _ => out.push(ch),
        }
    }
    if let Some(seq) = escape {
        out.push_str(&escape_marker(&seq));
    }
    out
}

fn caret_marker(ch: char) -> String {
    let code = ch as u32;
    if code < 0x20 {
        format!("^{}", char::from_u32(code + 0x40).unwrap_or('?'))
    } else {
        "^?".to_string()
    }
}

fn escape_marker(seq: &str) -> String {
    match seq {
        "[A" | "OA" | "[1;5A" => "↑".into(),
        "[B" | "OB" | "[1;5B" => "↓".into(),
        "[C" | "OC" | "[1;5C" => "→".into(),
        "[D" | "OD" | "[1;5D" => "←".into(),
        "[H" | "OH" | "[1~" | "[7~" => "⇱".into(),
        "[F" | "OF" | "[4~" | "[8~" => "⇲".into(),
        "[3~" => "⌦".into(),
        "[5~" => "⇞".into(),
        "[6~" => "⇟".into(),
        "[Z" => "⇧⇥".into(),
        // Alt/Meta + key: keep the key so no input is silently lost.
        _ if seq.chars().count() == 1 && !seq.starts_with('[') && !seq.starts_with('O') => {
            format!("⌥{seq}")
        }
        _ => "␛".into(),
    }
}

#[cfg(test)]
mod tests {
    use super::{AnsiStripper, InputLineBuffer, readable_command, strip_ansi};

    #[test]
    fn streaming_ansi_removal_survives_chunk_boundaries() {
        let text = "before\x1b[31mred\x1b[0m\x1b]0;title\x1b\\after\x1bPignored\x1b\\!";
        for boundary in 0..=text.len() {
            let mut parser = AnsiStripper::default();
            let output = parser.feed(&text[..boundary]) + &parser.feed(&text[boundary..]);
            assert_eq!(output, "beforeredafter!", "boundary {boundary}");
        }
    }

    #[test]
    fn strips_ansi_escapes() {
        let text = strip_ansi("\x1b]0;root@host:~\x07prompt> \x1b[1;32mok\x1b[0m\n");
        assert_eq!(text, "prompt> ok\n");

        let text = strip_ansi("\x1b[7;32m==>running...\x1b[0m\x1b[18;106H");
        assert_eq!(text, "==>running...");

        let text = strip_ansi("\x1b]0;GateShell\x1b\\x\x1b(B\x1b[?25l\x1b[?1049h");
        assert_eq!(text, "x");
    }

    #[test]
    fn strips_ss3_without_leaving_stray_letters() {
        assert_eq!(strip_ansi("\x1bOA\x1bOB\x1bOC\x1bOD"), "");
        assert_eq!(strip_ansi("ls\x1bOA\x1bOB\r\n"), "ls\r\n");
    }

    #[test]
    fn drops_noise_c0_but_keeps_cr_and_lf() {
        assert_eq!(strip_ansi("a\x07b\x08c"), "abc");
        assert_eq!(strip_ansi("first\r\nsecond\r\n"), "first\r\nsecond\r\n");
        assert_eq!(
            strip_ansi("progress 10%\rprogress 20%"),
            "progress 10%\rprogress 20%"
        );
        // Parser state survives chunk boundaries.
        let mut parser = AnsiStripper::default();
        assert_eq!(parser.feed("a\r") + &parser.feed("\nb"), "a\r\nb");
    }

    #[test]
    fn input_line_buffer_merges_keystrokes_into_one_command() {
        let mut buffer = InputLineBuffer::default();
        for key in ["l", "s", " ", "-", "l"] {
            assert!(buffer.feed(key).is_empty());
        }
        let lines = buffer.feed("\r");
        assert_eq!(lines.len(), 1);
        assert_eq!(lines[0].display, "ls -l");
        assert_eq!(lines[0].raw, "ls -l\r");
    }

    #[test]
    fn input_line_buffer_edits_and_marks_control_keys() {
        let mut buffer = InputLineBuffer::default();
        buffer.feed("pw");
        buffer.feed("\x7f");
        assert_eq!(buffer.feed("d\r")[0].display, "pd");

        let mut buffer = InputLineBuffer::default();
        assert!(
            buffer.feed("\x1b[A").is_empty(),
            "arrow key must not create a chunk"
        );
        assert_eq!(buffer.feed("\r")[0].display, "↑");

        let mut buffer = InputLineBuffer::default();
        assert!(
            buffer.feed("\x1bOB").is_empty(),
            "SS3 must not create a chunk"
        );
        assert_eq!(buffer.feed("\r")[0].display, "↓");

        let mut buffer = InputLineBuffer::default();
        buffer.feed("cd /us");
        assert_eq!(buffer.feed("\t\r")[0].display, "cd /us⇥");
    }

    #[test]
    fn input_line_buffer_handles_crlf_ctrl_c_and_split_escapes() {
        let mut buffer = InputLineBuffer::default();
        let lines = buffer.feed("pwd\r\n");
        assert_eq!(lines.len(), 1, "CRLF must submit only once");
        assert_eq!(lines[0].display, "pwd");

        let mut buffer = InputLineBuffer::default();
        assert!(buffer.feed("sleep 10").is_empty());
        assert_eq!(buffer.feed("\x03")[0].display, "sleep 10^C");

        let mut buffer = InputLineBuffer::default();
        buffer.feed("\x1b");
        buffer.feed("[");
        assert_eq!(buffer.feed("A\r")[0].display, "↑");
    }

    #[test]
    fn readable_command_keeps_multiline_scripts() {
        assert_eq!(
            readable_command("curl \\\n  --silent https://example.com"),
            "curl \\\n  --silent https://example.com"
        );
        assert_eq!(readable_command("a\x03b\x7fc"), "a^Cb^?c");
    }

    #[test]
    fn readable_command_marks_raw_keys() {
        assert_eq!(readable_command("\x1b[A\x1bOB"), "↑↓");
        assert_eq!(readable_command("\t"), "⇥");
        assert_eq!(readable_command("\x03"), "^C");
        assert_eq!(readable_command("echo\tOK"), "echo⇥OK");
        assert_eq!(readable_command(""), "");
        // An escape sequence split by a chunk boundary is not possible inside
        // one API call, but a dangling ESC must still be visible.
        assert_eq!(readable_command("\x1b"), "␛");
    }
}
