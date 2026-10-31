fn main() {
    println!("cargo:rustc-env=SHELLINK_TARGET={}", std::env::var("TARGET").unwrap());
    tauri_build::build()
}
