//! Shared terminal-text helpers for history storage, MCP tools, and events.

/// ANSI removal for a log stream. Parser state survives PTY read boundaries;
/// no screen grid, cursor emulation, or terminal rendering is performed.
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
}

impl AnsiStripper {
    pub fn feed(&mut self, text: &str) -> String {
        let mut out = String::with_capacity(text.len());
        for ch in text.chars() {
            self.state = match self.state {
                State::Ground => match ch {
                    '\x1b' => State::Escape,
                    '\u{9b}' => State::Csi,
                    '\u{9d}' => State::Osc,
                    _ => {
                        out.push(ch);
                        State::Ground
                    }
                },
                State::Escape => match ch {
                    '[' => State::Csi,
                    ']' => State::Osc,
                    'P' | 'X' | '^' | '_' => State::String,
                    '\x1b' => State::Escape,
                    '\u{20}'..='\u{2f}' => State::Intermediate,
                    _ => State::Ground,
                },
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

#[cfg(test)]
mod tests {
    use super::strip_ansi;

    #[test]
    fn streaming_ansi_removal_survives_chunk_boundaries() {
        let text = "before\x1b[31mred\x1b[0m\x1b]0;title\x1b\\after\x1bPignored\x1b\\!";
        for boundary in 0..=text.len() {
            let mut parser = super::AnsiStripper::default();
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
}
