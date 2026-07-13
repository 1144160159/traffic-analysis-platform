# HA Data Consistency Review Checklist

| Component | Before snapshot | After snapshot | Required consistency checks | Reviewer verdict |
|---|---|---|---|---|
| kafka | TBD | TBD | all_partitions_have_leader<br>isr_recovers_to_replication_factor<br>live_topic_catalog_unchanged<br>consumer_offsets_continue | TBD |
| flink | TBD | TBD | all_expected_jobs_return_running<br>latest_completed_checkpoint_advances_after_recovery<br>no_root_exceptions<br>output_tables_do_not_duplicate_drill_records | TBD |
| clickhouse | TBD | TBD | system_replicas_not_readonly<br>absolute_delay_returns_below_60s<br>queue_size_returns_below_100<br>drill_query_counts_match_before_after | TBD |
| postgresql | TBD | TBD | primary_or_promoted_primary_accepts_connections<br>replicas_resume_streaming<br>replay_lag_returns_to_zero_or_site_threshold<br>control_plane_crud_smoke_passes | TBD |
| minio | TBD | TBD | health_endpoint_recovers<br>pcap_object_head_or_verify_passes<br>presigned_download_still_matches_sha256<br>lifecycle_or_retention_config_unchanged | TBD |

Every row must be backed by immutable before/after snapshots before a formal report is written.
