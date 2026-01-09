// rust/probe-agent/xtask/src/main.rs
use std::process::Command;
use anyhow::{Result, Context};
use clap::{Parser, Subcommand};

#[derive(Parser)]
#[command(name = "xtask")]
#[command(about = "Task runner for probe-agent")]
struct Cli {
    #[command(subcommand)]
    command: Commands,
}

#[derive(Subcommand)]
enum Commands {
    /// Build eBPF programs
    BuildEbpf {
        #[arg(long)]
        release: bool,
    },
    /// Run all tests
    Test,
    /// Format code
    Fmt,
    /// Run clippy
    Clippy,
}

fn main() -> Result<()> {
    let cli = Cli::parse();

    match cli.command {
        Commands::BuildEbpf { release } => build_ebpf(release),
        Commands::Test => run_tests(),
        Commands::Fmt => run_fmt(),
        Commands::Clippy => run_clippy(),
    }
}

fn build_ebpf(release: bool) -> Result<()> {
    println!("Building eBPF programs...");
    
    let mut cmd = Command::new("cargo");
    cmd.current_dir("probe-agent/ebpf")
        .arg("build")
        .arg("--target")
        .arg("bpfel-unknown-none")
        .arg("-Z")
        .arg("build-std=core");
    
    if release {
        cmd.arg("--release");
    }
    
    let status = cmd.status()
        .context("Failed to execute cargo build for eBPF")?;
    
    if !status.success() {
        anyhow::bail!("eBPF build failed");
    }
    
    println!("✓ eBPF programs built successfully");
    Ok(())
}

fn run_tests() -> Result<()> {
    println!("Running tests...");
    
    let status = Command::new("cargo")
        .arg("test")
        .arg("--all")
        .status()
        .context("Failed to run tests")?;
    
    if !status.success() {
        anyhow::bail!("Tests failed");
    }
    
    println!("✓ All tests passed");
    Ok(())
}

fn run_fmt() -> Result<()> {
    println!("Formatting code...");
    
    let status = Command::new("cargo")
        .arg("fmt")
        .arg("--all")
        .status()
        .context("Failed to run cargo fmt")?;
    
    if !status.success() {
        anyhow::bail!("Formatting failed");
    }
    
    println!("✓ Code formatted");
    Ok(())
}

fn run_clippy() -> Result<()> {
    println!("Running clippy...");
    
    let status = Command::new("cargo")
        .arg("clippy")
        .arg("--all")
        .arg("--")
        .arg("-D")
        .arg("warnings")
        .status()
        .context("Failed to run clippy")?;
    
    if !status.success() {
        anyhow::bail!("Clippy found issues");
    }
    
    println!("✓ Clippy passed");
    Ok(())
}