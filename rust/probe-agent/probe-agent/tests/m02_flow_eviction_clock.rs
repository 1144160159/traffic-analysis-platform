use probe_agent::aggregator::{EvictionClock, EvictionClockError, EvictionClockMode};

#[test]
fn m02_flow_eviction_clock_matrix() {
    let live = EvictionClock::live();
    let live_now = live.eviction_now().unwrap();
    assert_eq!(live.mode(), EvictionClockMode::LiveProcessing);
    assert!(live_now.epoch_millis > 1_000_000_000_000);
    assert!(!live_now.end_of_input);

    let offline = EvictionClock::offline();
    assert_eq!(
        offline.eviction_now(),
        Err(EvictionClockError::MissingOfflineWatermark)
    );
    offline.observe_capture_micros(1_000_000);
    offline.observe_capture_micros(900_000);
    assert_eq!(offline.eviction_now().unwrap().epoch_millis, 1_000);
    assert!(!offline.eviction_now().unwrap().end_of_input);
    offline.mark_end_of_input();
    let eof = offline.eviction_now().unwrap();
    assert_eq!(eof.epoch_millis, 1_000);
    assert!(eof.end_of_input);
}
