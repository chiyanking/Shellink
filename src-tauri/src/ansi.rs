//! Shared terminal-text helpers for history storage, MCP tools, and events.

/// Remove ANSI escape sequences (CSI, OSC, charset selection, two-char escapes)
/// from raw PTY output.
pub fn strip_ansi(text: &str) -> String {
    let mut out = String::with_capacity(text.len());
    let mut chars = text.chars().peekable();
    while let Some(c) = chars.next() {
        if c != '\x1b' {
            out.push(c);
            continue;
        }
        match chars.peek() {
            Some('[') => {
                chars.next();
                // CSI: parameters+intermediates until final byte 0x40-0x7E
                while let Some(&c) = chars.peek() {
                    chars.next();
                    if ('\u{40}'..='\u{7e}').contains(&c) {
                        break;
                    }
                    if !('\u{20}'..='\u{3f}').contains(&c) {
                        break;
                    }
                }
            }
            Some(']') => {
                chars.next();
                // OSC: terminated by BEL or ST (ESC \)
                while let Some(c) = chars.next() {
                    if c == '\x07' {
                        break;
                    }
                    if c == '\x1b' && chars.peek() == Some(&'\\') {
                        chars.next();
                        break;
                    }
                }
            }
            Some(c) if ('\u{20}'..='\u{2f}').contains(c) => {
                // ESC with intermediates (e.g. charset selection ESC ( B):
                // consume intermediates then the final byte
                while let Some(&c) = chars.peek() {
                    if !('\u{20}'..='\u{2f}').contains(&c) {
                        break;
                    }
                    chars.next();
                }
                chars.next();
            }
            Some(_) => {
                // Two-char escapes (keypad modes, index, etc.)
                chars.next();
            }
            None => {}
        }
    }
    out
}

#[cfg(test)]
mod tests {
    use super::strip_ansi;

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
