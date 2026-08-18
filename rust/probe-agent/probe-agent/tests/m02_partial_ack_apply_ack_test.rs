use probe_agent::sender::{AckPartition, LocalCache};
use proto_gen::{EventHeader, FlowEvent, FlowItemDisposition, FlowItemResult};
use tempfile::TempDir;

fn event(id: &str) -> FlowEvent {
    FlowEvent {
        flow_id: id.to_owned(),
        header: Some(EventHeader {
            event_id: id.to_owned(),
            ..EventHeader::default()
        }),
        ..FlowEvent::default()
    }
}

fn result(index: u32, id: &str, disposition: FlowItemDisposition) -> FlowItemResult {
    FlowItemResult {
        input_index: index,
        event_id: id.to_owned(),
        disposition: disposition as i32,
        reason_code: if disposition == FlowItemDisposition::RejectedInvalid {
            "FLOW_SCHEMA_INVALID".to_owned()
        } else {
            String::new()
        },
        ack_scope: if matches!(
            disposition,
            FlowItemDisposition::KafkaAcked | FlowItemDisposition::DuplicateCommitted
        ) {
            "KAFKA_BROKER_DURABLE".to_owned()
        } else {
            String::new()
        },
    }
}

#[test]
fn local_cache_apply_ack_exact_set() -> anyhow::Result<()> {
    let temp = TempDir::new()?;
    let cache = LocalCache::new(temp.path(), 16)?;
    let saved = cache.save(&[event("accepted"), event("retry"), event("invalid")])?;
    let claimed = cache.claim(&saved)?;
    let mixed = AckPartition {
        response_revision: 1,
        // Deliberately shuffled: matching must use both index and identity.
        item_results: vec![
            result(2, "invalid", FlowItemDisposition::RejectedInvalid),
            result(0, "accepted", FlowItemDisposition::KafkaAcked),
            result(1, "retry", FlowItemDisposition::OutcomeUnknown),
        ],
    };
    let application = cache.apply_ack(&claimed.batch_ref, &mixed)?;
    assert_eq!(application.terminal, 1);
    assert_eq!(application.quarantined, 1);
    assert_eq!(application.pending, 1);
    assert!(!application.completed);

    let pending = cache.get_pending(10)?;
    assert_eq!(pending.len(), 1);
    assert_eq!(pending[0].1.len(), 1);
    assert_eq!(pending[0].1[0].header.as_ref().unwrap().event_id, "retry");

    let claimed = cache.claim(&pending[0].0)?;
    let malformed = AckPartition {
        response_revision: 1,
        item_results: vec![result(0, "foreign", FlowItemDisposition::KafkaAcked)],
    };
    assert!(cache.apply_ack(&claimed.batch_ref, &malformed).is_err());
    cache.release_claim(&claimed.batch_ref)?;
    assert_eq!(cache.get_pending(10)?[0].1.len(), 1);
    Ok(())
}
