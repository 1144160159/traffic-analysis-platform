// rust/probe-agent/probe-agent/build.rs
fn main() {
    // 编译时检查
    println!("cargo:rerun-if-changed=build.rs");
    
    // 检查 proto-gen 是否存在
    if !std::path::Path::new("../proto-gen/src/lib.rs").exists() {
        panic!("proto-gen not found! Please run proto generation first.");
    }
}