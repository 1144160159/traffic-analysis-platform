// rust/probe-agent/proto-gen/src/lib.rs
//! Generated protobuf types for Traffic Analysis Platform
//!
//! This crate contains auto-generated Rust types from the protobuf definitions.

#![allow(clippy::all)]
#![allow(unused_imports)]
#![allow(dead_code)]

/// Traffic v1 protobuf messages
pub mod traffic {
    pub mod v1 {
        include!("traffic.v1.rs");

        // Re-export commonly used types
        pub use self::event_header::*;
        pub use self::five_tuple::*;
        pub use self::flow_event::*;
        pub use self::session_event::*;
        pub use self::feature_stat_v1::*;
        pub use self::feature_seq_v1::*;
        pub use self::detection_event::*;
        pub use self::alert::*;
        pub use self::pcap_index_meta::*;
        pub use self::campaign::*;

        // gRPC client
        pub mod ingest_client {
            include!("traffic.v1.tonic.rs");
        }
    }
}

// 便捷导出
pub use traffic::v1::*;