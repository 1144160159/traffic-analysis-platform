// rust/probe-agent/xtask/src/main.rs
//! Build tasks for probe-agent

use anyhow::{Context, Result};
use clap::{Parser, Subcommand};
use std::path::PathBuf;
use std::process::Command;

#[derive(Parser)]
#[command(name = "xtask")]
#[command(about = "Build tasks for probe-agent")]
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
    /// Build the probe-agent binary
    Build {
        #[arg(long)]
        release: bool,
        #[arg(long, default_value = "x86_64-unknown-linux-gnu")]
        target: String,
    },
    /// Build everything (eBPF + agent)
    BuildAll {
        #[arg(long)]
        release: bool,
    },
    /// Run the probe-agent
    Run {
        #[arg(long, default_value = "config.yaml")]
        config: String,
    },
}

fn main() -> Result<()> {
    let cli = Cli::parse();

    match cli.command {
        Commands::BuildEbpf { release } => build_ebpf(release),
        Commands::Build { release, target } => build_agent(release, &target),
        Commands::BuildAll { release } => {
            build_ebpf(release)?;
            build_agent(release, "x86_64-unknown-linux-gnu")
        }
        Commands::Run { config } => run_agent(&config),
    }
}

// ========================================================================
// 🔧 修复 P10: 修复 eBPF 构建路径
// ========================================================================

fn build_ebpf(release: bool) -> Result<()> {
    println!("Building eBPF programs...");

    let workspace_root = workspace_root()?;

    // 🔧 修复：正确的 eBPF 路径
    let ebpf_dir = workspace_root.join("probe-agent").join("ebpf");

    if !ebpf_dir.exists() {
        anyhow::bail!(
            "eBPF directory not found: {}. Please check project structure.",
            ebpf_dir.display()
        );
    }

    let target = "bpfel-unknown-none";

    let mut cmd = Command::new("cargo");
    cmd.current_dir(&ebpf_dir)
        .arg("build")
        .arg("--target")
        .arg(target)
        .arg("-Z")
        .arg("build-std=core");

    if release {
        cmd.arg("--release");
    }

    let status = cmd.status().context("Failed to build eBPF")?;

    if !status.success() {
        anyhow::bail!("eBPF build failed");
    }

    // 🔧 修复：正确的输出文件路径
    let profile = if release { "release" } else { "debug" };
    let src = workspace_root
        .join("target")
        .join(target)
        .join(profile)
        .join("xdp_redirect");
    let dst = ebpf_dir.join("xdp_redirect.o");

    if src.exists() {
        std::fs::copy(&src, &dst)?;
        println!("✓ eBPF program copied to: {}", dst.display());
    } else {
        anyhow::bail!("eBPF binary not found at: {}", src.display());
    }

    println!("✓ eBPF build complete!");
    Ok(())
}

fn build_agent(release: bool, target: &str) -> Result<()> {
    println!("Building probe-agent for {}...", target);

    let workspace_root = workspace_root()?;

    let mut cmd = Command::new("cargo");
    cmd.current_dir(&workspace_root)
        .arg("build")
        .arg("--package")
        .arg("probe-agent")
        .arg("--target")
        .arg(target);

    if release {
        cmd.arg("--release");
    }

    let status = cmd.status().context("Failed to build probe-agent")?;

    if !status.success() {
        anyhow::bail!("Build failed");
    }

    let profile = if release { "release" } else { "debug" };
    let binary = workspace_root
        .join("target")
        .join(target)
        .join(profile)
        .join("probe-agent");

    println!("✓ Build complete: {}", binary.display());
    Ok(())
}

fn run_agent(config: &str) -> Result<()> {
    println!("Running probe-agent with config: {}", config);

    let workspace_root = workspace_root()?;

    let status = Command::new("cargo")
        .current_dir(&workspace_root)
        .arg("run")
        .arg("--package")
        .arg("probe-agent")
        .arg("--")
        .arg(config)
        .status()
        .context("Failed to run probe-agent")?;

    if !status.success() {
        anyhow::bail!("Run failed");
    }

    Ok(())
}

fn workspace_root() -> Result<PathBuf> {
    let output = Command::new("cargo")
        .arg("locate-project")
        .arg("--workspace")
        .arg("--message-format=plain")
        .output()
        .context("Failed to locate workspace")?;

    let path = String::from_utf8(output.stdout)?;
    let path = PathBuf::from(path.trim());

    // 返回 Cargo.toml 的父目录
    Ok(path.parent().unwrap().to_path_buf())
}
