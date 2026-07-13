use super::flow_table::TosUpdatePolicy;
use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct FlowTableConfig {
    pub capacity: usize,

    #[serde(default = "default_tos_policy")]
    pub tos_update_policy: String,

    #[serde(default = "default_enable_dscp_tracking")]
    pub enable_dscp_tracking: bool,
}

fn default_tos_policy() -> String {
    "highest_dscp".to_string()
}

fn default_enable_dscp_tracking() -> bool {
    true
}

impl Default for FlowTableConfig {
    fn default() -> Self {
        Self {
            capacity: 1_000_000,
            tos_update_policy: default_tos_policy(),
            enable_dscp_tracking: default_enable_dscp_tracking(),
        }
    }
}

impl FlowTableConfig {
    pub fn get_tos_policy(&self) -> TosUpdatePolicy {
        TosUpdatePolicy::from_str(&self.tos_update_policy).unwrap_or(TosUpdatePolicy::HighestDscp)
    }

    pub fn validate(&self) -> Result<(), String> {
        if self.capacity == 0 {
            return Err("capacity must be > 0".to_string());
        }

        if self.capacity > 100_000_000 {
            return Err("capacity exceeds maximum (100M)".to_string());
        }

        if TosUpdatePolicy::from_str(&self.tos_update_policy).is_none() {
            return Err(format!(
                "invalid tos_update_policy: '{}'. Valid values: first_non_zero, highest_dscp, last_seen, bitmap",
                self.tos_update_policy
            ));
        }

        Ok(())
    }

    pub fn example_yaml() -> &'static str {
        r#"
# Flow Table Configuration
flow_table:
  capacity: 1000000
  
  # ToS Update Policy:
  # - first_non_zero: Keep the first non-zero ToS value seen (legacy behavior)
  # - highest_dscp: Keep the highest DSCP value seen (recommended for QoS tracking)
  # - last_seen: Always update to the most recent ToS value
  # - bitmap: Track all unique DSCP values (uses additional 8 bytes per flow)
  tos_update_policy: highest_dscp
  
  # Enable tracking of all seen DSCP values (only effective with bitmap policy)
  enable_dscp_tracking: true
"#
    }
}
