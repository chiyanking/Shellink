mod bridge;
mod data;
use std::sync::Arc;
use tauri::{Manager, webview::PageLoadEvent};
#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    tauri::Builder::default()
        .manage(Arc::new(bridge::Backend::default()))
        .manage(data::DataStore::open().ok())
        .invoke_handler(tauri::generate_handler![bridge::core_request, bridge::core_reconnect, bridge::open_data_directory])
        .on_page_load(|webview, payload| {
            if webview.label() == "main" && matches!(payload.event(), PageLoadEvent::Finished) { let _ = webview.window().show(); }
        })
        .build(tauri::generate_context!())
        .expect("无法启动 Shellink")
        .run(|app, event| {
            if matches!(event, tauri::RunEvent::Exit) { app.state::<Arc<bridge::Backend>>().shutdown(app); }
        });
}
