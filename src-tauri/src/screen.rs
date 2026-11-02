//! Minimal VT screen emulator: replays raw terminal output into a character
//! grid so a snapshot shows the visible screen (what the user sees) instead of
//! the escape-sequence stream. Covers the subset needed by shells, progress
//! bars and full-screen apps (vim, top); colors, styles and OSC are ignored.

use vte::ansi::{ClearMode, Handler, LineClearMode, Processor, Timeout};

/// vte's default feature set is `no_std`, which removes `StdSyncHandler`.
/// Synchronized updates (CSI 2026) don't matter for a snapshot, so never
/// schedule a timeout.
#[derive(Default)]
struct NeverSync;

impl Timeout for NeverSync {
    fn set_timeout(&mut self, _: std::time::Duration) {}
    fn clear_timeout(&mut self) {}
    fn pending_timeout(&self) -> bool {
        false
    }
}

pub struct ScreenEmulator {
    cols: usize,
    rows: usize,
    grid: Vec<char>,
    cursor: usize, // linear index into grid
    saved_cursor: Option<usize>,
    pending_wrap: bool,
}

impl ScreenEmulator {
    pub fn new(cols: usize, rows: usize) -> Self {
        let cols = cols.max(1);
        let rows = rows.max(1);
        Self {
            cols,
            rows,
            grid: vec![' '; cols * rows],
            cursor: 0,
            saved_cursor: None,
            pending_wrap: false,
        }
    }

    /// Replay a chunk of raw terminal output into the grid.
    pub fn feed(&mut self, raw: &str) {
        let mut processor = Processor::<NeverSync>::new();
        for byte in raw.bytes() {
            processor.advance(self, byte);
        }
    }

    /// Grid position (0-based row, col) of the cursor.
    fn pos(&self) -> (usize, usize) {
        (self.cursor / self.cols, self.cursor % self.cols)
    }

    fn goto(&mut self, row: usize, col: usize) {
        self.cursor = row.min(self.rows - 1) * self.cols + col.min(self.cols - 1);
    }

    fn scroll_up(&mut self) {
        self.grid.drain(..self.cols);
        self.grid.extend(std::iter::repeat(' ').take(self.cols));
    }

    /// Move down one row, scrolling when already on the bottom row. Keeps the
    /// column (VT linefeed semantics) unless `cr` resets it to col 0.
    fn linefeed(&mut self, cr: bool) {
        let (row, col) = self.pos();
        let col = if cr { 0 } else { col };
        if row + 1 >= self.rows {
            self.scroll_up();
            self.cursor = (self.rows - 1) * self.cols + col;
        } else {
            self.cursor = (row + 1) * self.cols + col;
        }
    }

    /// Visible screen text, trailing blanks trimmed per line.
    pub fn snapshot(&self) -> String {
        (0..self.rows)
            .map(|row| {
                let line = &self.grid[row * self.cols..(row + 1) * self.cols];
                line.iter().collect::<String>().trim_end().to_owned()
            })
            .collect::<Vec<_>>()
            .join("\n")
    }
}

impl Handler for ScreenEmulator {
    fn input(&mut self, c: char) {
        if self.pending_wrap {
            self.linefeed(true);
            self.pending_wrap = false;
        }
        let (_, col) = self.pos();
        self.grid[self.cursor] = c;
        if col + 1 >= self.cols {
            // Park on the last column; autowrap happens on the next input.
            self.pending_wrap = true;
        } else {
            self.cursor += 1;
        }
    }

    fn linefeed(&mut self) {
        self.linefeed(false);
    }

    fn carriage_return(&mut self) {
        self.cursor = (self.cursor / self.cols) * self.cols;
    }

    fn backspace(&mut self) {
        if self.cursor % self.cols > 0 {
            self.cursor -= 1;
        }
    }

    fn put_tab(&mut self, count: u16) {
        for _ in 0..count.max(1) {
            let col = self.cursor % self.cols;
            let next = ((col / 8) + 1) * 8;
            self.cursor = (self.cursor / self.cols) * self.cols + next.min(self.cols - 1);
        }
    }

    fn goto(&mut self, line: i32, col: usize) {
        let row = line.max(0) as usize;
        self.goto(row, col);
    }

    fn goto_line(&mut self, line: i32) {
        let (_, col) = self.pos();
        self.goto(line.max(0) as usize, col);
    }

    fn goto_col(&mut self, col: usize) {
        let (row, _) = self.pos();
        self.goto(row, col);
    }

    fn move_up(&mut self, n: usize) {
        let (row, col) = self.pos();
        self.cursor = row.saturating_sub(n.max(1)) * self.cols + col;
    }

    fn move_down(&mut self, n: usize) {
        let (row, col) = self.pos();
        self.cursor = (row + n.max(1)).min(self.rows - 1) * self.cols + col;
    }

    fn move_forward(&mut self, col: usize) {
        let (row, c) = self.pos();
        self.cursor = row * self.cols + (c + col.max(1)).min(self.cols - 1);
    }

    fn move_backward(&mut self, col: usize) {
        let (row, c) = self.pos();
        self.cursor = row * self.cols + c.saturating_sub(col.max(1));
    }

    fn move_down_and_cr(&mut self, n: usize) {
        self.carriage_return();
        self.move_down(n);
    }

    fn move_up_and_cr(&mut self, n: usize) {
        self.carriage_return();
        self.move_up(n);
    }

    fn erase_chars(&mut self, count: usize) {
        for cell in self
            .grid[self.cursor..
                (self.cursor + count.max(1)).min(self.cols - self.cursor % self.cols + self.cursor)]
            .iter_mut()
        {
            *cell = ' ';
        }
    }

    fn clear_screen(&mut self, mode: ClearMode) {
        let (row, col) = self.pos();
        match mode {
            ClearMode::Below => {
                for cell in self.grid[self.cursor..].iter_mut() {
                    *cell = ' ';
                }
            }
            ClearMode::Above => {
                let line_end = row * self.cols + col + 1;
                for cell in self.grid[..line_end].iter_mut() {
                    *cell = ' ';
                }
            }
            ClearMode::All | ClearMode::Saved => {
                for cell in self.grid.iter_mut() {
                    *cell = ' ';
                }
            }
        }
    }

    fn clear_line(&mut self, mode: LineClearMode) {
        let (row, _) = self.pos();
        let line_start = row * self.cols;
        let line_end = line_start + self.cols;
        let range = match mode {
            LineClearMode::Right => self.cursor..line_end,
            LineClearMode::Left => line_start..self.cursor + 1,
            LineClearMode::All => line_start..line_end,
        };
        for cell in self.grid[range].iter_mut() {
            *cell = ' ';
        }
    }

    fn insert_blank_lines(&mut self, count: usize) {
        let count = count.max(1);
        let (row, col) = self.pos();
        let line_start = row * self.cols;
        let bottom = (line_start + count * self.cols).min(self.grid.len());
        self.grid.drain(line_start..bottom);
        self.grid
            .extend(std::iter::repeat(' ').take(bottom - line_start));
        self.cursor = row * self.cols + col;
    }

    fn delete_lines(&mut self, count: usize) {
        let count = count.max(1);
        let (row, col) = self.pos();
        let line_start = row * self.cols;
        let bottom = (line_start + count * self.cols).min(self.grid.len());
        self.grid.drain(line_start..bottom);
        self.grid
            .extend(std::iter::repeat(' ').take(bottom - line_start));
        self.cursor = row * self.cols + col;
    }

    fn insert_blank(&mut self, count: usize) {
        let (row, col) = self.pos();
        let end = row * self.cols + ((col + count.max(1)).min(self.cols));
        let shifted = end - col;
        self.grid.copy_within(col..end - shifted, end);
        for cell in self.grid[col..end - shifted].iter_mut() {
            *cell = ' ';
        }
    }

    fn delete_chars(&mut self, count: usize) {
        let (row, col) = self.pos();
        let end = row * self.cols + ((col + count.max(1)).min(self.cols));
        let shifted = end - col;
        self.grid.copy_within(end..row * self.cols + self.cols, col);
        for cell in self.grid[(row + 1) * self.cols - shifted..(row + 1) * self.cols].iter_mut() {
            *cell = ' ';
        }
    }

    fn save_cursor_position(&mut self) {
        self.saved_cursor = Some(self.cursor);
    }

    fn restore_cursor_position(&mut self) {
        if let Some(saved) = self.saved_cursor {
            self.cursor = saved.min(self.grid.len() - 1);
        }
    }

    fn reverse_index(&mut self) {
        // Move up, or scroll the screen down when at the top row.
        let (row, col) = self.pos();
        if row == 0 {
            self.grid.drain(self.grid.len() - self.cols..);
            self.grid.splice(..0, std::iter::repeat(' ').take(self.cols));
            self.cursor = col;
        } else {
            self.cursor = (row - 1) * self.cols + col;
        }
    }
}

#[cfg(test)]
mod tests {
    use super::{NeverSync, ScreenEmulator};
    use vte::ansi::Processor;

    fn snap(raw: &str, cols: usize, rows: usize) -> String {
        let mut screen = ScreenEmulator::new(cols, rows);
        let mut processor = Processor::<NeverSync>::new();
        for byte in raw.bytes() {
            processor.advance(&mut screen, byte);
        }
        screen.snapshot()
    }

    #[test]
    fn colored_prompt_is_plain_text() {
        let text = snap("\x1b]0;root@host:~\x07\x1b[1;32mroot@host\x1b[0m:~$ \x1b[1;32mok\x1b[0m\n", 80, 24);
        assert_eq!(text.split('\n').next().unwrap(), "root@host:~$ ok");
    }

    #[test]
    fn carriage_return_overwrites_progress_bar() {
        let text = snap("\x1b[7;32m==> 10%\x1b[0m\r==> 90%\x1b[0m\n", 80, 24);
        assert_eq!(text.split('\n').next().unwrap(), "==> 90%");
    }

    #[test]
    fn cursor_positioning_places_text_at_cell() {
        let text = snap("\x1b[2J\x1b[5;10Hhello", 80, 24);
        let line4 = text.split('\n').nth(4).unwrap();
        assert_eq!(line4.trim(), "hello");
        assert!(line4.starts_with("         h")); // col 10 => 9 spaces
    }

    #[test]
    fn newline_beyond_bottom_scrolls() {
        let mut raw = String::new();
        for i in 0..30 {
            raw.push_str(&format!("line{i}\r\n"));
        }
        let text = snap(&raw, 80, 24);
        let lines: Vec<&str> = text.split('\n').collect();
        assert_eq!(lines.len(), 24);
        // The trailing LF of line29 also scrolls at the bottom row, so line6
        // has left the screen: the window shows line7..line28 plus an empty row.
        assert_eq!(lines[0], "line7");
        assert_eq!(lines[22], "line29");
        assert_eq!(lines[23], "");
    }

    #[test]
    fn clear_line_on_status_redraw() {
        let text = snap(
            "\x1b[7;32m==>Is getting the instances, please wait patiently ...\x1b[0m\x1b[23;1H\x1b[1024D\x1b[K\x1b[7;32m==>done\x1b[0m",
            120,
            42,
        );
        assert_eq!(text.split('\n').nth(22).unwrap().trim_end(), "==>done");
    }

    #[test]
    fn backspace_edits_progress() {
        let text = snap("###-\x08=\x08=\n", 80, 24);
        assert_eq!(text.split('\n').next().unwrap(), "###=");
    }
}
