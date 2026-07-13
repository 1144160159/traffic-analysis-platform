fn main() {
    println!("cargo:rerun-if-changed=build.rs");

    if !std::path::Path::new("../proto-gen/src/lib.rs").exists() {
        panic!("proto-gen not found! Please run proto generation first.");
    }
}
