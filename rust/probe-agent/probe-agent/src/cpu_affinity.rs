use anyhow::{bail, Result};
use std::collections::HashSet;
use tracing::{debug, info, warn};

#[derive(Debug, Clone)]
pub struct CpuAffinityConfig {
    pub cpu_cores: Vec<u32>,

    pub numa_aware: bool,
}

impl Default for CpuAffinityConfig {
    fn default() -> Self {
        Self {
            cpu_cores: Vec::new(),
            numa_aware: false,
        }
    }
}

impl CpuAffinityConfig {
    pub fn new(cpu_cores: Vec<u32>) -> Self {
        Self {
            cpu_cores,
            numa_aware: false,
        }
    }

    pub fn numa_aware(cpu_cores: Vec<u32>) -> Self {
        Self {
            cpu_cores,
            numa_aware: true,
        }
    }

    pub fn validate(&self) -> Result<()> {
        if self.cpu_cores.is_empty() {
            return Ok(());
        }

        let num_cpus = num_cpus::get() as u32;

        for &cpu in &self.cpu_cores {
            if cpu >= num_cpus {
                bail!(
                    "Invalid CPU core {}: system has {} cores (0-{})",
                    cpu,
                    num_cpus,
                    num_cpus - 1
                );
            }
        }

        let unique: HashSet<_> = self.cpu_cores.iter().collect();
        if unique.len() != self.cpu_cores.len() {
            warn!("CPU core list contains duplicates, will be deduplicated");
        }

        Ok(())
    }

    pub fn is_enabled(&self) -> bool {
        !self.cpu_cores.is_empty()
    }
}

pub fn set_cpu_affinity(config: &CpuAffinityConfig) -> Result<()> {
    config.validate()?;

    if !config.is_enabled() {
        debug!("CPU affinity not configured, skipping");
        return Ok(());
    }

    info!(
        "Setting CPU affinity: cores={:?}, numa_aware={}",
        config.cpu_cores, config.numa_aware
    );

    let mut cpu_set = create_cpu_set(&config.cpu_cores)?;

    if config.numa_aware {
        cpu_set = optimize_for_numa(cpu_set, &config.cpu_cores)?;
    }

    apply_cpu_affinity(cpu_set)?;

    info!(
        "✓ CPU affinity set successfully to cores: {:?}",
        config.cpu_cores
    );

    Ok(())
}

pub fn set_thread_affinity(cpu_cores: &[u32]) -> Result<()> {
    if cpu_cores.is_empty() {
        return Ok(());
    }

    let cpu_set = create_cpu_set(cpu_cores)?;

    let ret =
        unsafe { libc::sched_setaffinity(0, std::mem::size_of::<libc::cpu_set_t>(), &cpu_set) };

    if ret != 0 {
        let err = std::io::Error::last_os_error();

        if err.raw_os_error() == Some(libc::EINVAL) {
            bail!("Invalid CPU cores: {:?}", cpu_cores);
        }

        if err.raw_os_error() == Some(libc::EPERM) {
            bail!(
                "Permission denied: Cannot set CPU affinity. \
                 Run with root or grant CAP_SYS_NICE capability."
            );
        }

        bail!("sched_setaffinity failed: {}", err);
    }

    debug!("Thread affinity set to cores: {:?}", cpu_cores);

    Ok(())
}

pub fn get_cpu_affinity() -> Result<Vec<u32>> {
    let mut cpu_set: libc::cpu_set_t = unsafe { std::mem::zeroed() };

    let ret =
        unsafe { libc::sched_getaffinity(0, std::mem::size_of::<libc::cpu_set_t>(), &mut cpu_set) };

    if ret != 0 {
        bail!(
            "sched_getaffinity failed: {}",
            std::io::Error::last_os_error()
        );
    }

    let mut cores = Vec::new();
    let num_cpus = num_cpus::get();

    for cpu in 0..num_cpus {
        if unsafe { libc::CPU_ISSET(cpu, &cpu_set) } {
            cores.push(cpu as u32);
        }
    }

    Ok(cores)
}

fn create_cpu_set(cpu_cores: &[u32]) -> Result<libc::cpu_set_t> {
    let mut cpu_set: libc::cpu_set_t = unsafe { std::mem::zeroed() };

    unsafe {
        libc::CPU_ZERO(&mut cpu_set);
    }

    for &cpu in cpu_cores {
        if cpu >= libc::CPU_SETSIZE as u32 {
            bail!(
                "CPU core {} exceeds maximum supported ({}) on this system",
                cpu,
                libc::CPU_SETSIZE - 1
            );
        }

        unsafe {
            libc::CPU_SET(cpu as usize, &mut cpu_set);
        }
    }

    Ok(cpu_set)
}

fn apply_cpu_affinity(cpu_set: libc::cpu_set_t) -> Result<()> {
    let ret =
        unsafe { libc::sched_setaffinity(0, std::mem::size_of::<libc::cpu_set_t>(), &cpu_set) };

    if ret != 0 {
        let err = std::io::Error::last_os_error();

        if err.raw_os_error() == Some(libc::EPERM) {
            bail!(
                "Permission denied: Cannot set CPU affinity. \
                 Run with root or grant CAP_SYS_NICE capability: \
                 sudo setcap cap_sys_nice+ep <binary>"
            );
        }

        bail!("sched_setaffinity failed: {}", err);
    }

    Ok(())
}

fn optimize_for_numa(cpu_set: libc::cpu_set_t, cpu_cores: &[u32]) -> Result<libc::cpu_set_t> {
    if !std::path::Path::new("/sys/devices/system/node").exists() {
        debug!("NUMA topology not available, skipping NUMA optimization");
        return Ok(cpu_set);
    }

    let numa_nodes = group_by_numa_node(cpu_cores);

    if numa_nodes.len() > 1 {
        warn!(
            "CPU cores span {} NUMA nodes: {:?}. \
             Consider using cores from a single node for best performance.",
            numa_nodes.len(),
            numa_nodes.keys().collect::<Vec<_>>()
        );
    } else if numa_nodes.len() == 1 {
        let node_id = numa_nodes.keys().next().unwrap();
        info!("✓ All CPU cores are on NUMA node {}", node_id);
    }

    Ok(cpu_set)
}

fn group_by_numa_node(cpu_cores: &[u32]) -> std::collections::HashMap<u32, Vec<u32>> {
    use std::collections::HashMap;

    let mut groups: HashMap<u32, Vec<u32>> = HashMap::new();

    for &cpu in cpu_cores {
        let node_id = get_numa_node_for_cpu(cpu).unwrap_or(0);
        groups.entry(node_id).or_insert_with(Vec::new).push(cpu);
    }

    groups
}

fn get_numa_node_for_cpu(cpu: u32) -> Option<u32> {
    let path = format!("/sys/devices/system/cpu/cpu{}/node", cpu);

    if let Ok(link) = std::fs::read_link(&path) {
        let node_name = link.file_name()?.to_str()?;
        if node_name.starts_with("node") {
            return node_name[4..].parse().ok();
        }
    }

    None
}

pub fn get_cpu_topology() -> CpuTopology {
    let num_cpus = num_cpus::get();
    let num_physical_cpus = num_cpus::get_physical();

    CpuTopology {
        total_cpus: num_cpus as u32,
        physical_cpus: num_physical_cpus as u32,
        numa_nodes: detect_numa_nodes(),
    }
}

#[derive(Debug, Clone)]
pub struct CpuTopology {
    pub total_cpus: u32,
    pub physical_cpus: u32,
    pub numa_nodes: Vec<NumaNode>,
}

#[derive(Debug, Clone)]
pub struct NumaNode {
    pub node_id: u32,
    pub cpu_cores: Vec<u32>,
}

fn detect_numa_nodes() -> Vec<NumaNode> {
    let mut nodes = Vec::new();

    let node_dir = std::path::Path::new("/sys/devices/system/node");
    if !node_dir.exists() {
        return nodes;
    }

    if let Ok(entries) = std::fs::read_dir(node_dir) {
        for entry in entries.flatten() {
            let name = entry.file_name();
            let name_str = name.to_string_lossy();

            if name_str.starts_with("node") {
                if let Ok(node_id) = name_str[4..].parse::<u32>() {
                    let cpu_list = read_cpu_list_for_node(node_id);
                    nodes.push(NumaNode {
                        node_id,
                        cpu_cores: cpu_list,
                    });
                }
            }
        }
    }

    nodes.sort_by_key(|n| n.node_id);
    nodes
}

fn read_cpu_list_for_node(node_id: u32) -> Vec<u32> {
    let path = format!("/sys/devices/system/node/node{}/cpulist", node_id);

    if let Ok(content) = std::fs::read_to_string(&path) {
        parse_cpu_list(&content.trim())
    } else {
        Vec::new()
    }
}

fn parse_cpu_list(list: &str) -> Vec<u32> {
    let mut cpus = Vec::new();

    for part in list.split(',') {
        if part.contains('-') {
            let range: Vec<&str> = part.split('-').collect();
            if range.len() == 2 {
                if let (Ok(start), Ok(end)) = (range[0].parse::<u32>(), range[1].parse::<u32>()) {
                    cpus.extend(start..=end);
                }
            }
        } else {
            if let Ok(cpu) = part.parse::<u32>() {
                cpus.push(cpu);
            }
        }
    }

    cpus
}
