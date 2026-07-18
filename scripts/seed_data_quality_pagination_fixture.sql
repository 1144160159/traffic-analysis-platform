-- Expand the active data-quality UI fixture so every server-backed table has
-- at least two pages of deterministic demo data. The first page remains
-- unchanged; the appended rows are marked as historical samples so page-two
-- interaction tests can prove that the row signature changed.
DO $$
DECLARE
  dataset TEXT;
  source_rows JSONB;
  historical_rows JSONB;
  seeded_payload JSONB;
  datasets TEXT[] := ARRAY[
    'consumerRows',
    'messageSizeTopicRows',
    'partitionQueueRows',
    'flinkJobRows',
    'flinkWindowRows',
    'flinkFailureRows',
    'fieldQualityRows',
    'communityCheckRows',
    'communityMismatchRows',
    'fieldAnomalyRows',
    'fieldLineageRows',
    'fieldRepairRows',
    'storageComponentRows',
    'storageFailureRows',
    'storageReplicaRows',
    'storagePartitionRows',
    'storageObjectRows',
    'replayTaskRows',
    'replayIdempotencyRows',
    'replayDifferenceRows',
    'replayEvidenceRows'
  ];
BEGIN
  SELECT payload
    INTO seeded_payload
    FROM data_quality_ui_fixtures
   WHERE tenant_id = 'default'
     AND active = TRUE
   FOR UPDATE;

  IF seeded_payload IS NULL THEN
    RAISE EXCEPTION 'active data-quality fixture for tenant default is required';
  END IF;

  IF seeded_payload ->> '_pagination_seed_version' = 'data-quality-pagination-v2' THEN
    RAISE NOTICE 'data-quality pagination fixture already seeded';
    RETURN;
  END IF;

  FOREACH dataset IN ARRAY datasets LOOP
    source_rows := seeded_payload -> dataset;
    IF jsonb_typeof(source_rows) <> 'array' OR jsonb_array_length(source_rows) = 0 THEN
      RAISE EXCEPTION 'fixture dataset % must be a non-empty array', dataset;
    END IF;

    SELECT jsonb_agg(
             CASE
               WHEN jsonb_typeof(item) = 'array' AND jsonb_array_length(item) > 0
                 THEN jsonb_set(item, '{0}', to_jsonb((item ->> 0) || ' · 历史样本'))
               ELSE item
             END
             ORDER BY ordinal
           )
      INTO historical_rows
      FROM jsonb_array_elements(source_rows) WITH ORDINALITY AS expanded(item, ordinal);

    seeded_payload := jsonb_set(seeded_payload, ARRAY[dataset], source_rows || historical_rows, TRUE);
  END LOOP;

  seeded_payload := jsonb_set(seeded_payload, '{_pagination_seed_version}', '"data-quality-pagination-v2"'::jsonb, TRUE);

  UPDATE data_quality_ui_fixtures
     SET payload = seeded_payload,
         fixture_version = 'data-quality-ui-v2-pagination',
         updated_at = NOW()
   WHERE tenant_id = 'default'
     AND active = TRUE;
END
$$;
