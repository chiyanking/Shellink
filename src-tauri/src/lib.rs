mod bridge;
mod data;
mod mcp;
mod sessions;
use std::sync::Arc;
use tauri::{Manager, webview::PageLoadEvent};
#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    let data_store = data::DataStore::open().expect("无法打开 Rust 数据库");
    tauri::Builder::default()
        .manage(Arc::new(bridge::Backend::default()))
        .manage(data_store)
        .manage(Arc::new(sessions::SessionManager::new()))
        .manage(mcp::Server::new())
        .setup(|app| {
            app.state::<mcp::Server>().start(app.handle().clone());
            Ok(())
        })
        .invoke_handler(tauri::generate_handler![
            bridge::core_request,
            bridge::open_data_directory
        ])
        .on_page_load(|webview, payload| {
            if webview.label() == "main" && matches!(payload.event(), PageLoadEvent::Finished) {
                let _ = webview.window().show();
            }
        })
        .on_window_event(|window, event| {
            if window.label() == "main"
                && matches!(event, tauri::WindowEvent::CloseRequested { .. })
            {
                window.app_handle().exit(0);
            }
        })
        .build(tauri::generate_context!())
        .expect("无法启动 Shellink")
        .run(|app, event| {
            if matches!(event, tauri::RunEvent::Exit) {
                app.state::<mcp::Server>().stop();
                app.state::<Arc<sessions::SessionManager>>()
                    .shutdown(&app.state::<data::DataStore>());
                app.state::<Arc<bridge::Backend>>().shutdown(app);
            }
        });
}

pub fn run_stdio() {
    mcp::run_stdio();
}
