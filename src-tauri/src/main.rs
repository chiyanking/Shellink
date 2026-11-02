// Prevents additional console window on Windows in release, DO NOT REMOVE!!
#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

fn main() {
    if std::env::var("SHELLINK_PROCESS_MODE").as_deref() == Ok("ssh-askpass") {
        std::process::exit(shellink_desktop_lib::run_askpass());
    }
    if std::env::args().nth(1).as_deref() == Some("mcp") {
        shellink_desktop_lib::run_stdio();
    } else {
        shellink_desktop_lib::run();
    }
}
