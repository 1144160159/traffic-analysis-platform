// @generated
impl serde::Serialize for ActiveIdleStats {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if self.min_ms != 0. {
            len += 1;
        }
        if self.mean_ms != 0. {
            len += 1;
        }
        if self.max_ms != 0. {
            len += 1;
        }
        if self.std_ms != 0. {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.ActiveIdleStats", len)?;
        if self.min_ms != 0. {
            struct_ser.serialize_field("minMs", &self.min_ms)?;
        }
        if self.mean_ms != 0. {
            struct_ser.serialize_field("meanMs", &self.mean_ms)?;
        }
        if self.max_ms != 0. {
            struct_ser.serialize_field("maxMs", &self.max_ms)?;
        }
        if self.std_ms != 0. {
            struct_ser.serialize_field("stdMs", &self.std_ms)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for ActiveIdleStats {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "min_ms",
            "minMs",
            "mean_ms",
            "meanMs",
            "max_ms",
            "maxMs",
            "std_ms",
            "stdMs",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            MinMs,
            MeanMs,
            MaxMs,
            StdMs,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "minMs" | "min_ms" => Ok(GeneratedField::MinMs),
                            "meanMs" | "mean_ms" => Ok(GeneratedField::MeanMs),
                            "maxMs" | "max_ms" => Ok(GeneratedField::MaxMs),
                            "stdMs" | "std_ms" => Ok(GeneratedField::StdMs),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = ActiveIdleStats;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.ActiveIdleStats")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<ActiveIdleStats, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut min_ms__ = None;
                let mut mean_ms__ = None;
                let mut max_ms__ = None;
                let mut std_ms__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::MinMs => {
                            if min_ms__.is_some() {
                                return Err(serde::de::Error::duplicate_field("minMs"));
                            }
                            min_ms__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::MeanMs => {
                            if mean_ms__.is_some() {
                                return Err(serde::de::Error::duplicate_field("meanMs"));
                            }
                            mean_ms__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::MaxMs => {
                            if max_ms__.is_some() {
                                return Err(serde::de::Error::duplicate_field("maxMs"));
                            }
                            max_ms__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::StdMs => {
                            if std_ms__.is_some() {
                                return Err(serde::de::Error::duplicate_field("stdMs"));
                            }
                            std_ms__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                    }
                }
                Ok(ActiveIdleStats {
                    min_ms: min_ms__.unwrap_or_default(),
                    mean_ms: mean_ms__.unwrap_or_default(),
                    max_ms: max_ms__.unwrap_or_default(),
                    std_ms: std_ms__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.ActiveIdleStats", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for Alert {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.tenant_id.is_empty() {
            len += 1;
        }
        if !self.alert_id.is_empty() {
            len += 1;
        }
        if self.first_seen != 0 {
            len += 1;
        }
        if self.last_seen != 0 {
            len += 1;
        }
        if self.severity != 0 {
            len += 1;
        }
        if !self.alert_type.is_empty() {
            len += 1;
        }
        if self.score != 0. {
            len += 1;
        }
        if !self.labels.is_empty() {
            len += 1;
        }
        if !self.src_ip.is_empty() {
            len += 1;
        }
        if !self.dst_ip.is_empty() {
            len += 1;
        }
        if self.src_port != 0 {
            len += 1;
        }
        if self.dst_port != 0 {
            len += 1;
        }
        if self.protocol != 0 {
            len += 1;
        }
        if !self.community_id.is_empty() {
            len += 1;
        }
        if !self.session_id.is_empty() {
            len += 1;
        }
        if !self.campaign_id.is_empty() {
            len += 1;
        }
        if !self.model_version.is_empty() {
            len += 1;
        }
        if !self.rule_version.is_empty() {
            len += 1;
        }
        if !self.feature_set_id.is_empty() {
            len += 1;
        }
        if self.status != 0 {
            len += 1;
        }
        if !self.assignee.is_empty() {
            len += 1;
        }
        if !self.evidence_ids.is_empty() {
            len += 1;
        }
        if !self.dedup_fingerprint.is_empty() {
            len += 1;
        }
        if self.updated_ts != 0 {
            len += 1;
        }
        if !self.event_id.is_empty() {
            len += 1;
        }
        if self.ingest_ts != 0 {
            len += 1;
        }
        if !self.protocol_name.is_empty() {
            len += 1;
        }
        if self.count != 0 {
            len += 1;
        }
        if !self.arkime_session_link.is_empty() {
            len += 1;
        }
        if !self.feedback_label.is_empty() {
            len += 1;
        }
        if self.feedback_count != 0 {
            len += 1;
        }
        if self.state_version != 0 {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.Alert", len)?;
        if !self.tenant_id.is_empty() {
            struct_ser.serialize_field("tenantId", &self.tenant_id)?;
        }
        if !self.alert_id.is_empty() {
            struct_ser.serialize_field("alertId", &self.alert_id)?;
        }
        if self.first_seen != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("firstSeen", ToString::to_string(&self.first_seen).as_str())?;
        }
        if self.last_seen != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("lastSeen", ToString::to_string(&self.last_seen).as_str())?;
        }
        if self.severity != 0 {
            let v = Severity::try_from(self.severity)
                .map_err(|_| serde::ser::Error::custom(format!("Invalid variant {}", self.severity)))?;
            struct_ser.serialize_field("severity", &v)?;
        }
        if !self.alert_type.is_empty() {
            struct_ser.serialize_field("alertType", &self.alert_type)?;
        }
        if self.score != 0. {
            struct_ser.serialize_field("score", &self.score)?;
        }
        if !self.labels.is_empty() {
            struct_ser.serialize_field("labels", &self.labels)?;
        }
        if !self.src_ip.is_empty() {
            struct_ser.serialize_field("srcIp", &self.src_ip)?;
        }
        if !self.dst_ip.is_empty() {
            struct_ser.serialize_field("dstIp", &self.dst_ip)?;
        }
        if self.src_port != 0 {
            struct_ser.serialize_field("srcPort", &self.src_port)?;
        }
        if self.dst_port != 0 {
            struct_ser.serialize_field("dstPort", &self.dst_port)?;
        }
        if self.protocol != 0 {
            struct_ser.serialize_field("protocol", &self.protocol)?;
        }
        if !self.community_id.is_empty() {
            struct_ser.serialize_field("communityId", &self.community_id)?;
        }
        if !self.session_id.is_empty() {
            struct_ser.serialize_field("sessionId", &self.session_id)?;
        }
        if !self.campaign_id.is_empty() {
            struct_ser.serialize_field("campaignId", &self.campaign_id)?;
        }
        if !self.model_version.is_empty() {
            struct_ser.serialize_field("modelVersion", &self.model_version)?;
        }
        if !self.rule_version.is_empty() {
            struct_ser.serialize_field("ruleVersion", &self.rule_version)?;
        }
        if !self.feature_set_id.is_empty() {
            struct_ser.serialize_field("featureSetId", &self.feature_set_id)?;
        }
        if self.status != 0 {
            let v = AlertStatus::try_from(self.status)
                .map_err(|_| serde::ser::Error::custom(format!("Invalid variant {}", self.status)))?;
            struct_ser.serialize_field("status", &v)?;
        }
        if !self.assignee.is_empty() {
            struct_ser.serialize_field("assignee", &self.assignee)?;
        }
        if !self.evidence_ids.is_empty() {
            struct_ser.serialize_field("evidenceIds", &self.evidence_ids)?;
        }
        if !self.dedup_fingerprint.is_empty() {
            struct_ser.serialize_field("dedupFingerprint", &self.dedup_fingerprint)?;
        }
        if self.updated_ts != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("updatedTs", ToString::to_string(&self.updated_ts).as_str())?;
        }
        if !self.event_id.is_empty() {
            struct_ser.serialize_field("eventId", &self.event_id)?;
        }
        if self.ingest_ts != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("ingestTs", ToString::to_string(&self.ingest_ts).as_str())?;
        }
        if !self.protocol_name.is_empty() {
            struct_ser.serialize_field("protocolName", &self.protocol_name)?;
        }
        if self.count != 0 {
            struct_ser.serialize_field("count", &self.count)?;
        }
        if !self.arkime_session_link.is_empty() {
            struct_ser.serialize_field("arkimeSessionLink", &self.arkime_session_link)?;
        }
        if !self.feedback_label.is_empty() {
            struct_ser.serialize_field("feedbackLabel", &self.feedback_label)?;
        }
        if self.feedback_count != 0 {
            struct_ser.serialize_field("feedbackCount", &self.feedback_count)?;
        }
        if self.state_version != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("stateVersion", ToString::to_string(&self.state_version).as_str())?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for Alert {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "tenant_id",
            "tenantId",
            "alert_id",
            "alertId",
            "first_seen",
            "firstSeen",
            "last_seen",
            "lastSeen",
            "severity",
            "alert_type",
            "alertType",
            "score",
            "labels",
            "src_ip",
            "srcIp",
            "dst_ip",
            "dstIp",
            "src_port",
            "srcPort",
            "dst_port",
            "dstPort",
            "protocol",
            "community_id",
            "communityId",
            "session_id",
            "sessionId",
            "campaign_id",
            "campaignId",
            "model_version",
            "modelVersion",
            "rule_version",
            "ruleVersion",
            "feature_set_id",
            "featureSetId",
            "status",
            "assignee",
            "evidence_ids",
            "evidenceIds",
            "dedup_fingerprint",
            "dedupFingerprint",
            "updated_ts",
            "updatedTs",
            "event_id",
            "eventId",
            "ingest_ts",
            "ingestTs",
            "protocol_name",
            "protocolName",
            "count",
            "arkime_session_link",
            "arkimeSessionLink",
            "feedback_label",
            "feedbackLabel",
            "feedback_count",
            "feedbackCount",
            "state_version",
            "stateVersion",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            TenantId,
            AlertId,
            FirstSeen,
            LastSeen,
            Severity,
            AlertType,
            Score,
            Labels,
            SrcIp,
            DstIp,
            SrcPort,
            DstPort,
            Protocol,
            CommunityId,
            SessionId,
            CampaignId,
            ModelVersion,
            RuleVersion,
            FeatureSetId,
            Status,
            Assignee,
            EvidenceIds,
            DedupFingerprint,
            UpdatedTs,
            EventId,
            IngestTs,
            ProtocolName,
            Count,
            ArkimeSessionLink,
            FeedbackLabel,
            FeedbackCount,
            StateVersion,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "tenantId" | "tenant_id" => Ok(GeneratedField::TenantId),
                            "alertId" | "alert_id" => Ok(GeneratedField::AlertId),
                            "firstSeen" | "first_seen" => Ok(GeneratedField::FirstSeen),
                            "lastSeen" | "last_seen" => Ok(GeneratedField::LastSeen),
                            "severity" => Ok(GeneratedField::Severity),
                            "alertType" | "alert_type" => Ok(GeneratedField::AlertType),
                            "score" => Ok(GeneratedField::Score),
                            "labels" => Ok(GeneratedField::Labels),
                            "srcIp" | "src_ip" => Ok(GeneratedField::SrcIp),
                            "dstIp" | "dst_ip" => Ok(GeneratedField::DstIp),
                            "srcPort" | "src_port" => Ok(GeneratedField::SrcPort),
                            "dstPort" | "dst_port" => Ok(GeneratedField::DstPort),
                            "protocol" => Ok(GeneratedField::Protocol),
                            "communityId" | "community_id" => Ok(GeneratedField::CommunityId),
                            "sessionId" | "session_id" => Ok(GeneratedField::SessionId),
                            "campaignId" | "campaign_id" => Ok(GeneratedField::CampaignId),
                            "modelVersion" | "model_version" => Ok(GeneratedField::ModelVersion),
                            "ruleVersion" | "rule_version" => Ok(GeneratedField::RuleVersion),
                            "featureSetId" | "feature_set_id" => Ok(GeneratedField::FeatureSetId),
                            "status" => Ok(GeneratedField::Status),
                            "assignee" => Ok(GeneratedField::Assignee),
                            "evidenceIds" | "evidence_ids" => Ok(GeneratedField::EvidenceIds),
                            "dedupFingerprint" | "dedup_fingerprint" => Ok(GeneratedField::DedupFingerprint),
                            "updatedTs" | "updated_ts" => Ok(GeneratedField::UpdatedTs),
                            "eventId" | "event_id" => Ok(GeneratedField::EventId),
                            "ingestTs" | "ingest_ts" => Ok(GeneratedField::IngestTs),
                            "protocolName" | "protocol_name" => Ok(GeneratedField::ProtocolName),
                            "count" => Ok(GeneratedField::Count),
                            "arkimeSessionLink" | "arkime_session_link" => Ok(GeneratedField::ArkimeSessionLink),
                            "feedbackLabel" | "feedback_label" => Ok(GeneratedField::FeedbackLabel),
                            "feedbackCount" | "feedback_count" => Ok(GeneratedField::FeedbackCount),
                            "stateVersion" | "state_version" => Ok(GeneratedField::StateVersion),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = Alert;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.Alert")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<Alert, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut tenant_id__ = None;
                let mut alert_id__ = None;
                let mut first_seen__ = None;
                let mut last_seen__ = None;
                let mut severity__ = None;
                let mut alert_type__ = None;
                let mut score__ = None;
                let mut labels__ = None;
                let mut src_ip__ = None;
                let mut dst_ip__ = None;
                let mut src_port__ = None;
                let mut dst_port__ = None;
                let mut protocol__ = None;
                let mut community_id__ = None;
                let mut session_id__ = None;
                let mut campaign_id__ = None;
                let mut model_version__ = None;
                let mut rule_version__ = None;
                let mut feature_set_id__ = None;
                let mut status__ = None;
                let mut assignee__ = None;
                let mut evidence_ids__ = None;
                let mut dedup_fingerprint__ = None;
                let mut updated_ts__ = None;
                let mut event_id__ = None;
                let mut ingest_ts__ = None;
                let mut protocol_name__ = None;
                let mut count__ = None;
                let mut arkime_session_link__ = None;
                let mut feedback_label__ = None;
                let mut feedback_count__ = None;
                let mut state_version__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::TenantId => {
                            if tenant_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("tenantId"));
                            }
                            tenant_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::AlertId => {
                            if alert_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("alertId"));
                            }
                            alert_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::FirstSeen => {
                            if first_seen__.is_some() {
                                return Err(serde::de::Error::duplicate_field("firstSeen"));
                            }
                            first_seen__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::LastSeen => {
                            if last_seen__.is_some() {
                                return Err(serde::de::Error::duplicate_field("lastSeen"));
                            }
                            last_seen__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::Severity => {
                            if severity__.is_some() {
                                return Err(serde::de::Error::duplicate_field("severity"));
                            }
                            severity__ = Some(map_.next_value::<Severity>()? as i32);
                        }
                        GeneratedField::AlertType => {
                            if alert_type__.is_some() {
                                return Err(serde::de::Error::duplicate_field("alertType"));
                            }
                            alert_type__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Score => {
                            if score__.is_some() {
                                return Err(serde::de::Error::duplicate_field("score"));
                            }
                            score__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::Labels => {
                            if labels__.is_some() {
                                return Err(serde::de::Error::duplicate_field("labels"));
                            }
                            labels__ = Some(map_.next_value()?);
                        }
                        GeneratedField::SrcIp => {
                            if src_ip__.is_some() {
                                return Err(serde::de::Error::duplicate_field("srcIp"));
                            }
                            src_ip__ = Some(map_.next_value()?);
                        }
                        GeneratedField::DstIp => {
                            if dst_ip__.is_some() {
                                return Err(serde::de::Error::duplicate_field("dstIp"));
                            }
                            dst_ip__ = Some(map_.next_value()?);
                        }
                        GeneratedField::SrcPort => {
                            if src_port__.is_some() {
                                return Err(serde::de::Error::duplicate_field("srcPort"));
                            }
                            src_port__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::DstPort => {
                            if dst_port__.is_some() {
                                return Err(serde::de::Error::duplicate_field("dstPort"));
                            }
                            dst_port__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::Protocol => {
                            if protocol__.is_some() {
                                return Err(serde::de::Error::duplicate_field("protocol"));
                            }
                            protocol__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::CommunityId => {
                            if community_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("communityId"));
                            }
                            community_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::SessionId => {
                            if session_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("sessionId"));
                            }
                            session_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::CampaignId => {
                            if campaign_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("campaignId"));
                            }
                            campaign_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::ModelVersion => {
                            if model_version__.is_some() {
                                return Err(serde::de::Error::duplicate_field("modelVersion"));
                            }
                            model_version__ = Some(map_.next_value()?);
                        }
                        GeneratedField::RuleVersion => {
                            if rule_version__.is_some() {
                                return Err(serde::de::Error::duplicate_field("ruleVersion"));
                            }
                            rule_version__ = Some(map_.next_value()?);
                        }
                        GeneratedField::FeatureSetId => {
                            if feature_set_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("featureSetId"));
                            }
                            feature_set_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Status => {
                            if status__.is_some() {
                                return Err(serde::de::Error::duplicate_field("status"));
                            }
                            status__ = Some(map_.next_value::<AlertStatus>()? as i32);
                        }
                        GeneratedField::Assignee => {
                            if assignee__.is_some() {
                                return Err(serde::de::Error::duplicate_field("assignee"));
                            }
                            assignee__ = Some(map_.next_value()?);
                        }
                        GeneratedField::EvidenceIds => {
                            if evidence_ids__.is_some() {
                                return Err(serde::de::Error::duplicate_field("evidenceIds"));
                            }
                            evidence_ids__ = Some(map_.next_value()?);
                        }
                        GeneratedField::DedupFingerprint => {
                            if dedup_fingerprint__.is_some() {
                                return Err(serde::de::Error::duplicate_field("dedupFingerprint"));
                            }
                            dedup_fingerprint__ = Some(map_.next_value()?);
                        }
                        GeneratedField::UpdatedTs => {
                            if updated_ts__.is_some() {
                                return Err(serde::de::Error::duplicate_field("updatedTs"));
                            }
                            updated_ts__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::EventId => {
                            if event_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("eventId"));
                            }
                            event_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::IngestTs => {
                            if ingest_ts__.is_some() {
                                return Err(serde::de::Error::duplicate_field("ingestTs"));
                            }
                            ingest_ts__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::ProtocolName => {
                            if protocol_name__.is_some() {
                                return Err(serde::de::Error::duplicate_field("protocolName"));
                            }
                            protocol_name__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Count => {
                            if count__.is_some() {
                                return Err(serde::de::Error::duplicate_field("count"));
                            }
                            count__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::ArkimeSessionLink => {
                            if arkime_session_link__.is_some() {
                                return Err(serde::de::Error::duplicate_field("arkimeSessionLink"));
                            }
                            arkime_session_link__ = Some(map_.next_value()?);
                        }
                        GeneratedField::FeedbackLabel => {
                            if feedback_label__.is_some() {
                                return Err(serde::de::Error::duplicate_field("feedbackLabel"));
                            }
                            feedback_label__ = Some(map_.next_value()?);
                        }
                        GeneratedField::FeedbackCount => {
                            if feedback_count__.is_some() {
                                return Err(serde::de::Error::duplicate_field("feedbackCount"));
                            }
                            feedback_count__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::StateVersion => {
                            if state_version__.is_some() {
                                return Err(serde::de::Error::duplicate_field("stateVersion"));
                            }
                            state_version__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                    }
                }
                Ok(Alert {
                    tenant_id: tenant_id__.unwrap_or_default(),
                    alert_id: alert_id__.unwrap_or_default(),
                    first_seen: first_seen__.unwrap_or_default(),
                    last_seen: last_seen__.unwrap_or_default(),
                    severity: severity__.unwrap_or_default(),
                    alert_type: alert_type__.unwrap_or_default(),
                    score: score__.unwrap_or_default(),
                    labels: labels__.unwrap_or_default(),
                    src_ip: src_ip__.unwrap_or_default(),
                    dst_ip: dst_ip__.unwrap_or_default(),
                    src_port: src_port__.unwrap_or_default(),
                    dst_port: dst_port__.unwrap_or_default(),
                    protocol: protocol__.unwrap_or_default(),
                    community_id: community_id__.unwrap_or_default(),
                    session_id: session_id__.unwrap_or_default(),
                    campaign_id: campaign_id__.unwrap_or_default(),
                    model_version: model_version__.unwrap_or_default(),
                    rule_version: rule_version__.unwrap_or_default(),
                    feature_set_id: feature_set_id__.unwrap_or_default(),
                    status: status__.unwrap_or_default(),
                    assignee: assignee__.unwrap_or_default(),
                    evidence_ids: evidence_ids__.unwrap_or_default(),
                    dedup_fingerprint: dedup_fingerprint__.unwrap_or_default(),
                    updated_ts: updated_ts__.unwrap_or_default(),
                    event_id: event_id__.unwrap_or_default(),
                    ingest_ts: ingest_ts__.unwrap_or_default(),
                    protocol_name: protocol_name__.unwrap_or_default(),
                    count: count__.unwrap_or_default(),
                    arkime_session_link: arkime_session_link__.unwrap_or_default(),
                    feedback_label: feedback_label__.unwrap_or_default(),
                    feedback_count: feedback_count__.unwrap_or_default(),
                    state_version: state_version__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.Alert", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for AlertBatch {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.alerts.is_empty() {
            len += 1;
        }
        if !self.evidences.is_empty() {
            len += 1;
        }
        if !self.campaigns.is_empty() {
            len += 1;
        }
        if !self.batch_id.is_empty() {
            len += 1;
        }
        if !self.tenant_id.is_empty() {
            len += 1;
        }
        if self.created_at != 0 {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.AlertBatch", len)?;
        if !self.alerts.is_empty() {
            struct_ser.serialize_field("alerts", &self.alerts)?;
        }
        if !self.evidences.is_empty() {
            struct_ser.serialize_field("evidences", &self.evidences)?;
        }
        if !self.campaigns.is_empty() {
            struct_ser.serialize_field("campaigns", &self.campaigns)?;
        }
        if !self.batch_id.is_empty() {
            struct_ser.serialize_field("batchId", &self.batch_id)?;
        }
        if !self.tenant_id.is_empty() {
            struct_ser.serialize_field("tenantId", &self.tenant_id)?;
        }
        if self.created_at != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("createdAt", ToString::to_string(&self.created_at).as_str())?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for AlertBatch {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "alerts",
            "evidences",
            "campaigns",
            "batch_id",
            "batchId",
            "tenant_id",
            "tenantId",
            "created_at",
            "createdAt",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Alerts,
            Evidences,
            Campaigns,
            BatchId,
            TenantId,
            CreatedAt,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "alerts" => Ok(GeneratedField::Alerts),
                            "evidences" => Ok(GeneratedField::Evidences),
                            "campaigns" => Ok(GeneratedField::Campaigns),
                            "batchId" | "batch_id" => Ok(GeneratedField::BatchId),
                            "tenantId" | "tenant_id" => Ok(GeneratedField::TenantId),
                            "createdAt" | "created_at" => Ok(GeneratedField::CreatedAt),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = AlertBatch;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.AlertBatch")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<AlertBatch, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut alerts__ = None;
                let mut evidences__ = None;
                let mut campaigns__ = None;
                let mut batch_id__ = None;
                let mut tenant_id__ = None;
                let mut created_at__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Alerts => {
                            if alerts__.is_some() {
                                return Err(serde::de::Error::duplicate_field("alerts"));
                            }
                            alerts__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Evidences => {
                            if evidences__.is_some() {
                                return Err(serde::de::Error::duplicate_field("evidences"));
                            }
                            evidences__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Campaigns => {
                            if campaigns__.is_some() {
                                return Err(serde::de::Error::duplicate_field("campaigns"));
                            }
                            campaigns__ = Some(map_.next_value()?);
                        }
                        GeneratedField::BatchId => {
                            if batch_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("batchId"));
                            }
                            batch_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::TenantId => {
                            if tenant_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("tenantId"));
                            }
                            tenant_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::CreatedAt => {
                            if created_at__.is_some() {
                                return Err(serde::de::Error::duplicate_field("createdAt"));
                            }
                            created_at__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                    }
                }
                Ok(AlertBatch {
                    alerts: alerts__.unwrap_or_default(),
                    evidences: evidences__.unwrap_or_default(),
                    campaigns: campaigns__.unwrap_or_default(),
                    batch_id: batch_id__.unwrap_or_default(),
                    tenant_id: tenant_id__.unwrap_or_default(),
                    created_at: created_at__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.AlertBatch", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for AlertCorrelationEdge {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.tenant_id.is_empty() {
            len += 1;
        }
        if !self.edge_id.is_empty() {
            len += 1;
        }
        if !self.source_alert_id.is_empty() {
            len += 1;
        }
        if !self.target_alert_id.is_empty() {
            len += 1;
        }
        if !self.correlation_type.is_empty() {
            len += 1;
        }
        if self.correlation_score != 0. {
            len += 1;
        }
        if !self.shared_entities.is_empty() {
            len += 1;
        }
        if self.time_delta_ms != 0 {
            len += 1;
        }
        if self.ts != 0 {
            len += 1;
        }
        if self.ingest_ts != 0 {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.AlertCorrelationEdge", len)?;
        if !self.tenant_id.is_empty() {
            struct_ser.serialize_field("tenantId", &self.tenant_id)?;
        }
        if !self.edge_id.is_empty() {
            struct_ser.serialize_field("edgeId", &self.edge_id)?;
        }
        if !self.source_alert_id.is_empty() {
            struct_ser.serialize_field("sourceAlertId", &self.source_alert_id)?;
        }
        if !self.target_alert_id.is_empty() {
            struct_ser.serialize_field("targetAlertId", &self.target_alert_id)?;
        }
        if !self.correlation_type.is_empty() {
            struct_ser.serialize_field("correlationType", &self.correlation_type)?;
        }
        if self.correlation_score != 0. {
            struct_ser.serialize_field("correlationScore", &self.correlation_score)?;
        }
        if !self.shared_entities.is_empty() {
            struct_ser.serialize_field("sharedEntities", &self.shared_entities)?;
        }
        if self.time_delta_ms != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("timeDeltaMs", ToString::to_string(&self.time_delta_ms).as_str())?;
        }
        if self.ts != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("ts", ToString::to_string(&self.ts).as_str())?;
        }
        if self.ingest_ts != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("ingestTs", ToString::to_string(&self.ingest_ts).as_str())?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for AlertCorrelationEdge {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "tenant_id",
            "tenantId",
            "edge_id",
            "edgeId",
            "source_alert_id",
            "sourceAlertId",
            "target_alert_id",
            "targetAlertId",
            "correlation_type",
            "correlationType",
            "correlation_score",
            "correlationScore",
            "shared_entities",
            "sharedEntities",
            "time_delta_ms",
            "timeDeltaMs",
            "ts",
            "ingest_ts",
            "ingestTs",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            TenantId,
            EdgeId,
            SourceAlertId,
            TargetAlertId,
            CorrelationType,
            CorrelationScore,
            SharedEntities,
            TimeDeltaMs,
            Ts,
            IngestTs,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "tenantId" | "tenant_id" => Ok(GeneratedField::TenantId),
                            "edgeId" | "edge_id" => Ok(GeneratedField::EdgeId),
                            "sourceAlertId" | "source_alert_id" => Ok(GeneratedField::SourceAlertId),
                            "targetAlertId" | "target_alert_id" => Ok(GeneratedField::TargetAlertId),
                            "correlationType" | "correlation_type" => Ok(GeneratedField::CorrelationType),
                            "correlationScore" | "correlation_score" => Ok(GeneratedField::CorrelationScore),
                            "sharedEntities" | "shared_entities" => Ok(GeneratedField::SharedEntities),
                            "timeDeltaMs" | "time_delta_ms" => Ok(GeneratedField::TimeDeltaMs),
                            "ts" => Ok(GeneratedField::Ts),
                            "ingestTs" | "ingest_ts" => Ok(GeneratedField::IngestTs),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = AlertCorrelationEdge;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.AlertCorrelationEdge")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<AlertCorrelationEdge, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut tenant_id__ = None;
                let mut edge_id__ = None;
                let mut source_alert_id__ = None;
                let mut target_alert_id__ = None;
                let mut correlation_type__ = None;
                let mut correlation_score__ = None;
                let mut shared_entities__ = None;
                let mut time_delta_ms__ = None;
                let mut ts__ = None;
                let mut ingest_ts__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::TenantId => {
                            if tenant_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("tenantId"));
                            }
                            tenant_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::EdgeId => {
                            if edge_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("edgeId"));
                            }
                            edge_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::SourceAlertId => {
                            if source_alert_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("sourceAlertId"));
                            }
                            source_alert_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::TargetAlertId => {
                            if target_alert_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("targetAlertId"));
                            }
                            target_alert_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::CorrelationType => {
                            if correlation_type__.is_some() {
                                return Err(serde::de::Error::duplicate_field("correlationType"));
                            }
                            correlation_type__ = Some(map_.next_value()?);
                        }
                        GeneratedField::CorrelationScore => {
                            if correlation_score__.is_some() {
                                return Err(serde::de::Error::duplicate_field("correlationScore"));
                            }
                            correlation_score__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::SharedEntities => {
                            if shared_entities__.is_some() {
                                return Err(serde::de::Error::duplicate_field("sharedEntities"));
                            }
                            shared_entities__ = Some(map_.next_value()?);
                        }
                        GeneratedField::TimeDeltaMs => {
                            if time_delta_ms__.is_some() {
                                return Err(serde::de::Error::duplicate_field("timeDeltaMs"));
                            }
                            time_delta_ms__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::Ts => {
                            if ts__.is_some() {
                                return Err(serde::de::Error::duplicate_field("ts"));
                            }
                            ts__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::IngestTs => {
                            if ingest_ts__.is_some() {
                                return Err(serde::de::Error::duplicate_field("ingestTs"));
                            }
                            ingest_ts__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                    }
                }
                Ok(AlertCorrelationEdge {
                    tenant_id: tenant_id__.unwrap_or_default(),
                    edge_id: edge_id__.unwrap_or_default(),
                    source_alert_id: source_alert_id__.unwrap_or_default(),
                    target_alert_id: target_alert_id__.unwrap_or_default(),
                    correlation_type: correlation_type__.unwrap_or_default(),
                    correlation_score: correlation_score__.unwrap_or_default(),
                    shared_entities: shared_entities__.unwrap_or_default(),
                    time_delta_ms: time_delta_ms__.unwrap_or_default(),
                    ts: ts__.unwrap_or_default(),
                    ingest_ts: ingest_ts__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.AlertCorrelationEdge", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for AlertExtendedBatch {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.batch_id.is_empty() {
            len += 1;
        }
        if !self.tenant_id.is_empty() {
            len += 1;
        }
        if self.created_at != 0 {
            len += 1;
        }
        if !self.alerts.is_empty() {
            len += 1;
        }
        if !self.evidences.is_empty() {
            len += 1;
        }
        if !self.campaigns.is_empty() {
            len += 1;
        }
        if !self.feedbacks.is_empty() {
            len += 1;
        }
        if !self.whitelist_rules.is_empty() {
            len += 1;
        }
        if !self.state_transitions.is_empty() {
            len += 1;
        }
        if !self.dedup_stats.is_empty() {
            len += 1;
        }
        if !self.storage_health_events.is_empty() {
            len += 1;
        }
        if !self.model_feedback_metrics.is_empty() {
            len += 1;
        }
        if !self.correlation_edges.is_empty() {
            len += 1;
        }
        if !self.notification_events.is_empty() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.AlertExtendedBatch", len)?;
        if !self.batch_id.is_empty() {
            struct_ser.serialize_field("batchId", &self.batch_id)?;
        }
        if !self.tenant_id.is_empty() {
            struct_ser.serialize_field("tenantId", &self.tenant_id)?;
        }
        if self.created_at != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("createdAt", ToString::to_string(&self.created_at).as_str())?;
        }
        if !self.alerts.is_empty() {
            struct_ser.serialize_field("alerts", &self.alerts)?;
        }
        if !self.evidences.is_empty() {
            struct_ser.serialize_field("evidences", &self.evidences)?;
        }
        if !self.campaigns.is_empty() {
            struct_ser.serialize_field("campaigns", &self.campaigns)?;
        }
        if !self.feedbacks.is_empty() {
            struct_ser.serialize_field("feedbacks", &self.feedbacks)?;
        }
        if !self.whitelist_rules.is_empty() {
            struct_ser.serialize_field("whitelistRules", &self.whitelist_rules)?;
        }
        if !self.state_transitions.is_empty() {
            struct_ser.serialize_field("stateTransitions", &self.state_transitions)?;
        }
        if !self.dedup_stats.is_empty() {
            struct_ser.serialize_field("dedupStats", &self.dedup_stats)?;
        }
        if !self.storage_health_events.is_empty() {
            struct_ser.serialize_field("storageHealthEvents", &self.storage_health_events)?;
        }
        if !self.model_feedback_metrics.is_empty() {
            struct_ser.serialize_field("modelFeedbackMetrics", &self.model_feedback_metrics)?;
        }
        if !self.correlation_edges.is_empty() {
            struct_ser.serialize_field("correlationEdges", &self.correlation_edges)?;
        }
        if !self.notification_events.is_empty() {
            struct_ser.serialize_field("notificationEvents", &self.notification_events)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for AlertExtendedBatch {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "batch_id",
            "batchId",
            "tenant_id",
            "tenantId",
            "created_at",
            "createdAt",
            "alerts",
            "evidences",
            "campaigns",
            "feedbacks",
            "whitelist_rules",
            "whitelistRules",
            "state_transitions",
            "stateTransitions",
            "dedup_stats",
            "dedupStats",
            "storage_health_events",
            "storageHealthEvents",
            "model_feedback_metrics",
            "modelFeedbackMetrics",
            "correlation_edges",
            "correlationEdges",
            "notification_events",
            "notificationEvents",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            BatchId,
            TenantId,
            CreatedAt,
            Alerts,
            Evidences,
            Campaigns,
            Feedbacks,
            WhitelistRules,
            StateTransitions,
            DedupStats,
            StorageHealthEvents,
            ModelFeedbackMetrics,
            CorrelationEdges,
            NotificationEvents,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "batchId" | "batch_id" => Ok(GeneratedField::BatchId),
                            "tenantId" | "tenant_id" => Ok(GeneratedField::TenantId),
                            "createdAt" | "created_at" => Ok(GeneratedField::CreatedAt),
                            "alerts" => Ok(GeneratedField::Alerts),
                            "evidences" => Ok(GeneratedField::Evidences),
                            "campaigns" => Ok(GeneratedField::Campaigns),
                            "feedbacks" => Ok(GeneratedField::Feedbacks),
                            "whitelistRules" | "whitelist_rules" => Ok(GeneratedField::WhitelistRules),
                            "stateTransitions" | "state_transitions" => Ok(GeneratedField::StateTransitions),
                            "dedupStats" | "dedup_stats" => Ok(GeneratedField::DedupStats),
                            "storageHealthEvents" | "storage_health_events" => Ok(GeneratedField::StorageHealthEvents),
                            "modelFeedbackMetrics" | "model_feedback_metrics" => Ok(GeneratedField::ModelFeedbackMetrics),
                            "correlationEdges" | "correlation_edges" => Ok(GeneratedField::CorrelationEdges),
                            "notificationEvents" | "notification_events" => Ok(GeneratedField::NotificationEvents),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = AlertExtendedBatch;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.AlertExtendedBatch")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<AlertExtendedBatch, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut batch_id__ = None;
                let mut tenant_id__ = None;
                let mut created_at__ = None;
                let mut alerts__ = None;
                let mut evidences__ = None;
                let mut campaigns__ = None;
                let mut feedbacks__ = None;
                let mut whitelist_rules__ = None;
                let mut state_transitions__ = None;
                let mut dedup_stats__ = None;
                let mut storage_health_events__ = None;
                let mut model_feedback_metrics__ = None;
                let mut correlation_edges__ = None;
                let mut notification_events__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::BatchId => {
                            if batch_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("batchId"));
                            }
                            batch_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::TenantId => {
                            if tenant_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("tenantId"));
                            }
                            tenant_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::CreatedAt => {
                            if created_at__.is_some() {
                                return Err(serde::de::Error::duplicate_field("createdAt"));
                            }
                            created_at__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::Alerts => {
                            if alerts__.is_some() {
                                return Err(serde::de::Error::duplicate_field("alerts"));
                            }
                            alerts__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Evidences => {
                            if evidences__.is_some() {
                                return Err(serde::de::Error::duplicate_field("evidences"));
                            }
                            evidences__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Campaigns => {
                            if campaigns__.is_some() {
                                return Err(serde::de::Error::duplicate_field("campaigns"));
                            }
                            campaigns__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Feedbacks => {
                            if feedbacks__.is_some() {
                                return Err(serde::de::Error::duplicate_field("feedbacks"));
                            }
                            feedbacks__ = Some(map_.next_value()?);
                        }
                        GeneratedField::WhitelistRules => {
                            if whitelist_rules__.is_some() {
                                return Err(serde::de::Error::duplicate_field("whitelistRules"));
                            }
                            whitelist_rules__ = Some(map_.next_value()?);
                        }
                        GeneratedField::StateTransitions => {
                            if state_transitions__.is_some() {
                                return Err(serde::de::Error::duplicate_field("stateTransitions"));
                            }
                            state_transitions__ = Some(map_.next_value()?);
                        }
                        GeneratedField::DedupStats => {
                            if dedup_stats__.is_some() {
                                return Err(serde::de::Error::duplicate_field("dedupStats"));
                            }
                            dedup_stats__ = Some(map_.next_value()?);
                        }
                        GeneratedField::StorageHealthEvents => {
                            if storage_health_events__.is_some() {
                                return Err(serde::de::Error::duplicate_field("storageHealthEvents"));
                            }
                            storage_health_events__ = Some(map_.next_value()?);
                        }
                        GeneratedField::ModelFeedbackMetrics => {
                            if model_feedback_metrics__.is_some() {
                                return Err(serde::de::Error::duplicate_field("modelFeedbackMetrics"));
                            }
                            model_feedback_metrics__ = Some(map_.next_value()?);
                        }
                        GeneratedField::CorrelationEdges => {
                            if correlation_edges__.is_some() {
                                return Err(serde::de::Error::duplicate_field("correlationEdges"));
                            }
                            correlation_edges__ = Some(map_.next_value()?);
                        }
                        GeneratedField::NotificationEvents => {
                            if notification_events__.is_some() {
                                return Err(serde::de::Error::duplicate_field("notificationEvents"));
                            }
                            notification_events__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(AlertExtendedBatch {
                    batch_id: batch_id__.unwrap_or_default(),
                    tenant_id: tenant_id__.unwrap_or_default(),
                    created_at: created_at__.unwrap_or_default(),
                    alerts: alerts__.unwrap_or_default(),
                    evidences: evidences__.unwrap_or_default(),
                    campaigns: campaigns__.unwrap_or_default(),
                    feedbacks: feedbacks__.unwrap_or_default(),
                    whitelist_rules: whitelist_rules__.unwrap_or_default(),
                    state_transitions: state_transitions__.unwrap_or_default(),
                    dedup_stats: dedup_stats__.unwrap_or_default(),
                    storage_health_events: storage_health_events__.unwrap_or_default(),
                    model_feedback_metrics: model_feedback_metrics__.unwrap_or_default(),
                    correlation_edges: correlation_edges__.unwrap_or_default(),
                    notification_events: notification_events__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.AlertExtendedBatch", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for AlertFeedback {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.tenant_id.is_empty() {
            len += 1;
        }
        if !self.feedback_id.is_empty() {
            len += 1;
        }
        if !self.alert_id.is_empty() {
            len += 1;
        }
        if !self.user_id.is_empty() {
            len += 1;
        }
        if !self.label.is_empty() {
            len += 1;
        }
        if !self.reason_code.is_empty() {
            len += 1;
        }
        if !self.comment.is_empty() {
            len += 1;
        }
        if self.add_to_whitelist != 0 {
            len += 1;
        }
        if !self.alert_type.is_empty() {
            len += 1;
        }
        if !self.severity.is_empty() {
            len += 1;
        }
        if !self.model_version.is_empty() {
            len += 1;
        }
        if !self.rule_version.is_empty() {
            len += 1;
        }
        if self.ts != 0 {
            len += 1;
        }
        if self.ingest_ts != 0 {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.AlertFeedback", len)?;
        if !self.tenant_id.is_empty() {
            struct_ser.serialize_field("tenantId", &self.tenant_id)?;
        }
        if !self.feedback_id.is_empty() {
            struct_ser.serialize_field("feedbackId", &self.feedback_id)?;
        }
        if !self.alert_id.is_empty() {
            struct_ser.serialize_field("alertId", &self.alert_id)?;
        }
        if !self.user_id.is_empty() {
            struct_ser.serialize_field("userId", &self.user_id)?;
        }
        if !self.label.is_empty() {
            struct_ser.serialize_field("label", &self.label)?;
        }
        if !self.reason_code.is_empty() {
            struct_ser.serialize_field("reasonCode", &self.reason_code)?;
        }
        if !self.comment.is_empty() {
            struct_ser.serialize_field("comment", &self.comment)?;
        }
        if self.add_to_whitelist != 0 {
            struct_ser.serialize_field("addToWhitelist", &self.add_to_whitelist)?;
        }
        if !self.alert_type.is_empty() {
            struct_ser.serialize_field("alertType", &self.alert_type)?;
        }
        if !self.severity.is_empty() {
            struct_ser.serialize_field("severity", &self.severity)?;
        }
        if !self.model_version.is_empty() {
            struct_ser.serialize_field("modelVersion", &self.model_version)?;
        }
        if !self.rule_version.is_empty() {
            struct_ser.serialize_field("ruleVersion", &self.rule_version)?;
        }
        if self.ts != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("ts", ToString::to_string(&self.ts).as_str())?;
        }
        if self.ingest_ts != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("ingestTs", ToString::to_string(&self.ingest_ts).as_str())?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for AlertFeedback {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "tenant_id",
            "tenantId",
            "feedback_id",
            "feedbackId",
            "alert_id",
            "alertId",
            "user_id",
            "userId",
            "label",
            "reason_code",
            "reasonCode",
            "comment",
            "add_to_whitelist",
            "addToWhitelist",
            "alert_type",
            "alertType",
            "severity",
            "model_version",
            "modelVersion",
            "rule_version",
            "ruleVersion",
            "ts",
            "ingest_ts",
            "ingestTs",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            TenantId,
            FeedbackId,
            AlertId,
            UserId,
            Label,
            ReasonCode,
            Comment,
            AddToWhitelist,
            AlertType,
            Severity,
            ModelVersion,
            RuleVersion,
            Ts,
            IngestTs,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "tenantId" | "tenant_id" => Ok(GeneratedField::TenantId),
                            "feedbackId" | "feedback_id" => Ok(GeneratedField::FeedbackId),
                            "alertId" | "alert_id" => Ok(GeneratedField::AlertId),
                            "userId" | "user_id" => Ok(GeneratedField::UserId),
                            "label" => Ok(GeneratedField::Label),
                            "reasonCode" | "reason_code" => Ok(GeneratedField::ReasonCode),
                            "comment" => Ok(GeneratedField::Comment),
                            "addToWhitelist" | "add_to_whitelist" => Ok(GeneratedField::AddToWhitelist),
                            "alertType" | "alert_type" => Ok(GeneratedField::AlertType),
                            "severity" => Ok(GeneratedField::Severity),
                            "modelVersion" | "model_version" => Ok(GeneratedField::ModelVersion),
                            "ruleVersion" | "rule_version" => Ok(GeneratedField::RuleVersion),
                            "ts" => Ok(GeneratedField::Ts),
                            "ingestTs" | "ingest_ts" => Ok(GeneratedField::IngestTs),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = AlertFeedback;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.AlertFeedback")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<AlertFeedback, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut tenant_id__ = None;
                let mut feedback_id__ = None;
                let mut alert_id__ = None;
                let mut user_id__ = None;
                let mut label__ = None;
                let mut reason_code__ = None;
                let mut comment__ = None;
                let mut add_to_whitelist__ = None;
                let mut alert_type__ = None;
                let mut severity__ = None;
                let mut model_version__ = None;
                let mut rule_version__ = None;
                let mut ts__ = None;
                let mut ingest_ts__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::TenantId => {
                            if tenant_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("tenantId"));
                            }
                            tenant_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::FeedbackId => {
                            if feedback_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("feedbackId"));
                            }
                            feedback_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::AlertId => {
                            if alert_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("alertId"));
                            }
                            alert_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::UserId => {
                            if user_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("userId"));
                            }
                            user_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Label => {
                            if label__.is_some() {
                                return Err(serde::de::Error::duplicate_field("label"));
                            }
                            label__ = Some(map_.next_value()?);
                        }
                        GeneratedField::ReasonCode => {
                            if reason_code__.is_some() {
                                return Err(serde::de::Error::duplicate_field("reasonCode"));
                            }
                            reason_code__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Comment => {
                            if comment__.is_some() {
                                return Err(serde::de::Error::duplicate_field("comment"));
                            }
                            comment__ = Some(map_.next_value()?);
                        }
                        GeneratedField::AddToWhitelist => {
                            if add_to_whitelist__.is_some() {
                                return Err(serde::de::Error::duplicate_field("addToWhitelist"));
                            }
                            add_to_whitelist__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::AlertType => {
                            if alert_type__.is_some() {
                                return Err(serde::de::Error::duplicate_field("alertType"));
                            }
                            alert_type__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Severity => {
                            if severity__.is_some() {
                                return Err(serde::de::Error::duplicate_field("severity"));
                            }
                            severity__ = Some(map_.next_value()?);
                        }
                        GeneratedField::ModelVersion => {
                            if model_version__.is_some() {
                                return Err(serde::de::Error::duplicate_field("modelVersion"));
                            }
                            model_version__ = Some(map_.next_value()?);
                        }
                        GeneratedField::RuleVersion => {
                            if rule_version__.is_some() {
                                return Err(serde::de::Error::duplicate_field("ruleVersion"));
                            }
                            rule_version__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Ts => {
                            if ts__.is_some() {
                                return Err(serde::de::Error::duplicate_field("ts"));
                            }
                            ts__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::IngestTs => {
                            if ingest_ts__.is_some() {
                                return Err(serde::de::Error::duplicate_field("ingestTs"));
                            }
                            ingest_ts__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                    }
                }
                Ok(AlertFeedback {
                    tenant_id: tenant_id__.unwrap_or_default(),
                    feedback_id: feedback_id__.unwrap_or_default(),
                    alert_id: alert_id__.unwrap_or_default(),
                    user_id: user_id__.unwrap_or_default(),
                    label: label__.unwrap_or_default(),
                    reason_code: reason_code__.unwrap_or_default(),
                    comment: comment__.unwrap_or_default(),
                    add_to_whitelist: add_to_whitelist__.unwrap_or_default(),
                    alert_type: alert_type__.unwrap_or_default(),
                    severity: severity__.unwrap_or_default(),
                    model_version: model_version__.unwrap_or_default(),
                    rule_version: rule_version__.unwrap_or_default(),
                    ts: ts__.unwrap_or_default(),
                    ingest_ts: ingest_ts__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.AlertFeedback", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for AlertStateTransition {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.tenant_id.is_empty() {
            len += 1;
        }
        if !self.alert_id.is_empty() {
            len += 1;
        }
        if !self.transition_id.is_empty() {
            len += 1;
        }
        if !self.old_status.is_empty() {
            len += 1;
        }
        if !self.new_status.is_empty() {
            len += 1;
        }
        if !self.old_assignee.is_empty() {
            len += 1;
        }
        if !self.new_assignee.is_empty() {
            len += 1;
        }
        if !self.changed_by.is_empty() {
            len += 1;
        }
        if !self.change_reason.is_empty() {
            len += 1;
        }
        if self.state_version != 0 {
            len += 1;
        }
        if self.ts != 0 {
            len += 1;
        }
        if self.ingest_ts != 0 {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.AlertStateTransition", len)?;
        if !self.tenant_id.is_empty() {
            struct_ser.serialize_field("tenantId", &self.tenant_id)?;
        }
        if !self.alert_id.is_empty() {
            struct_ser.serialize_field("alertId", &self.alert_id)?;
        }
        if !self.transition_id.is_empty() {
            struct_ser.serialize_field("transitionId", &self.transition_id)?;
        }
        if !self.old_status.is_empty() {
            struct_ser.serialize_field("oldStatus", &self.old_status)?;
        }
        if !self.new_status.is_empty() {
            struct_ser.serialize_field("newStatus", &self.new_status)?;
        }
        if !self.old_assignee.is_empty() {
            struct_ser.serialize_field("oldAssignee", &self.old_assignee)?;
        }
        if !self.new_assignee.is_empty() {
            struct_ser.serialize_field("newAssignee", &self.new_assignee)?;
        }
        if !self.changed_by.is_empty() {
            struct_ser.serialize_field("changedBy", &self.changed_by)?;
        }
        if !self.change_reason.is_empty() {
            struct_ser.serialize_field("changeReason", &self.change_reason)?;
        }
        if self.state_version != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("stateVersion", ToString::to_string(&self.state_version).as_str())?;
        }
        if self.ts != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("ts", ToString::to_string(&self.ts).as_str())?;
        }
        if self.ingest_ts != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("ingestTs", ToString::to_string(&self.ingest_ts).as_str())?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for AlertStateTransition {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "tenant_id",
            "tenantId",
            "alert_id",
            "alertId",
            "transition_id",
            "transitionId",
            "old_status",
            "oldStatus",
            "new_status",
            "newStatus",
            "old_assignee",
            "oldAssignee",
            "new_assignee",
            "newAssignee",
            "changed_by",
            "changedBy",
            "change_reason",
            "changeReason",
            "state_version",
            "stateVersion",
            "ts",
            "ingest_ts",
            "ingestTs",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            TenantId,
            AlertId,
            TransitionId,
            OldStatus,
            NewStatus,
            OldAssignee,
            NewAssignee,
            ChangedBy,
            ChangeReason,
            StateVersion,
            Ts,
            IngestTs,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "tenantId" | "tenant_id" => Ok(GeneratedField::TenantId),
                            "alertId" | "alert_id" => Ok(GeneratedField::AlertId),
                            "transitionId" | "transition_id" => Ok(GeneratedField::TransitionId),
                            "oldStatus" | "old_status" => Ok(GeneratedField::OldStatus),
                            "newStatus" | "new_status" => Ok(GeneratedField::NewStatus),
                            "oldAssignee" | "old_assignee" => Ok(GeneratedField::OldAssignee),
                            "newAssignee" | "new_assignee" => Ok(GeneratedField::NewAssignee),
                            "changedBy" | "changed_by" => Ok(GeneratedField::ChangedBy),
                            "changeReason" | "change_reason" => Ok(GeneratedField::ChangeReason),
                            "stateVersion" | "state_version" => Ok(GeneratedField::StateVersion),
                            "ts" => Ok(GeneratedField::Ts),
                            "ingestTs" | "ingest_ts" => Ok(GeneratedField::IngestTs),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = AlertStateTransition;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.AlertStateTransition")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<AlertStateTransition, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut tenant_id__ = None;
                let mut alert_id__ = None;
                let mut transition_id__ = None;
                let mut old_status__ = None;
                let mut new_status__ = None;
                let mut old_assignee__ = None;
                let mut new_assignee__ = None;
                let mut changed_by__ = None;
                let mut change_reason__ = None;
                let mut state_version__ = None;
                let mut ts__ = None;
                let mut ingest_ts__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::TenantId => {
                            if tenant_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("tenantId"));
                            }
                            tenant_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::AlertId => {
                            if alert_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("alertId"));
                            }
                            alert_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::TransitionId => {
                            if transition_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("transitionId"));
                            }
                            transition_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::OldStatus => {
                            if old_status__.is_some() {
                                return Err(serde::de::Error::duplicate_field("oldStatus"));
                            }
                            old_status__ = Some(map_.next_value()?);
                        }
                        GeneratedField::NewStatus => {
                            if new_status__.is_some() {
                                return Err(serde::de::Error::duplicate_field("newStatus"));
                            }
                            new_status__ = Some(map_.next_value()?);
                        }
                        GeneratedField::OldAssignee => {
                            if old_assignee__.is_some() {
                                return Err(serde::de::Error::duplicate_field("oldAssignee"));
                            }
                            old_assignee__ = Some(map_.next_value()?);
                        }
                        GeneratedField::NewAssignee => {
                            if new_assignee__.is_some() {
                                return Err(serde::de::Error::duplicate_field("newAssignee"));
                            }
                            new_assignee__ = Some(map_.next_value()?);
                        }
                        GeneratedField::ChangedBy => {
                            if changed_by__.is_some() {
                                return Err(serde::de::Error::duplicate_field("changedBy"));
                            }
                            changed_by__ = Some(map_.next_value()?);
                        }
                        GeneratedField::ChangeReason => {
                            if change_reason__.is_some() {
                                return Err(serde::de::Error::duplicate_field("changeReason"));
                            }
                            change_reason__ = Some(map_.next_value()?);
                        }
                        GeneratedField::StateVersion => {
                            if state_version__.is_some() {
                                return Err(serde::de::Error::duplicate_field("stateVersion"));
                            }
                            state_version__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::Ts => {
                            if ts__.is_some() {
                                return Err(serde::de::Error::duplicate_field("ts"));
                            }
                            ts__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::IngestTs => {
                            if ingest_ts__.is_some() {
                                return Err(serde::de::Error::duplicate_field("ingestTs"));
                            }
                            ingest_ts__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                    }
                }
                Ok(AlertStateTransition {
                    tenant_id: tenant_id__.unwrap_or_default(),
                    alert_id: alert_id__.unwrap_or_default(),
                    transition_id: transition_id__.unwrap_or_default(),
                    old_status: old_status__.unwrap_or_default(),
                    new_status: new_status__.unwrap_or_default(),
                    old_assignee: old_assignee__.unwrap_or_default(),
                    new_assignee: new_assignee__.unwrap_or_default(),
                    changed_by: changed_by__.unwrap_or_default(),
                    change_reason: change_reason__.unwrap_or_default(),
                    state_version: state_version__.unwrap_or_default(),
                    ts: ts__.unwrap_or_default(),
                    ingest_ts: ingest_ts__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.AlertStateTransition", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for AlertStatus {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        let variant = match self {
            Self::Unspecified => "ALERT_STATUS_UNSPECIFIED",
            Self::New => "ALERT_STATUS_NEW",
            Self::Triage => "ALERT_STATUS_TRIAGE",
            Self::Assigned => "ALERT_STATUS_ASSIGNED",
            Self::InProgress => "ALERT_STATUS_IN_PROGRESS",
            Self::Resolved => "ALERT_STATUS_RESOLVED",
            Self::Closed => "ALERT_STATUS_CLOSED",
            Self::FalsePositive => "ALERT_STATUS_FALSE_POSITIVE",
        };
        serializer.serialize_str(variant)
    }
}
impl<'de> serde::Deserialize<'de> for AlertStatus {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "ALERT_STATUS_UNSPECIFIED",
            "ALERT_STATUS_NEW",
            "ALERT_STATUS_TRIAGE",
            "ALERT_STATUS_ASSIGNED",
            "ALERT_STATUS_IN_PROGRESS",
            "ALERT_STATUS_RESOLVED",
            "ALERT_STATUS_CLOSED",
            "ALERT_STATUS_FALSE_POSITIVE",
        ];

        struct GeneratedVisitor;

        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = AlertStatus;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                write!(formatter, "expected one of: {:?}", &FIELDS)
            }

            fn visit_i64<E>(self, v: i64) -> std::result::Result<Self::Value, E>
            where
                E: serde::de::Error,
            {
                i32::try_from(v)
                    .ok()
                    .and_then(|x| x.try_into().ok())
                    .ok_or_else(|| {
                        serde::de::Error::invalid_value(serde::de::Unexpected::Signed(v), &self)
                    })
            }

            fn visit_u64<E>(self, v: u64) -> std::result::Result<Self::Value, E>
            where
                E: serde::de::Error,
            {
                i32::try_from(v)
                    .ok()
                    .and_then(|x| x.try_into().ok())
                    .ok_or_else(|| {
                        serde::de::Error::invalid_value(serde::de::Unexpected::Unsigned(v), &self)
                    })
            }

            fn visit_str<E>(self, value: &str) -> std::result::Result<Self::Value, E>
            where
                E: serde::de::Error,
            {
                match value {
                    "ALERT_STATUS_UNSPECIFIED" => Ok(AlertStatus::Unspecified),
                    "ALERT_STATUS_NEW" => Ok(AlertStatus::New),
                    "ALERT_STATUS_TRIAGE" => Ok(AlertStatus::Triage),
                    "ALERT_STATUS_ASSIGNED" => Ok(AlertStatus::Assigned),
                    "ALERT_STATUS_IN_PROGRESS" => Ok(AlertStatus::InProgress),
                    "ALERT_STATUS_RESOLVED" => Ok(AlertStatus::Resolved),
                    "ALERT_STATUS_CLOSED" => Ok(AlertStatus::Closed),
                    "ALERT_STATUS_FALSE_POSITIVE" => Ok(AlertStatus::FalsePositive),
                    _ => Err(serde::de::Error::unknown_variant(value, FIELDS)),
                }
            }
        }
        deserializer.deserialize_any(GeneratedVisitor)
    }
}
impl serde::Serialize for AlertUpdate {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.alert_id.is_empty() {
            len += 1;
        }
        if self.status != 0 {
            len += 1;
        }
        if !self.assignee.is_empty() {
            len += 1;
        }
        if !self.comment.is_empty() {
            len += 1;
        }
        if !self.updated_by.is_empty() {
            len += 1;
        }
        if self.updated_at != 0 {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.AlertUpdate", len)?;
        if !self.alert_id.is_empty() {
            struct_ser.serialize_field("alertId", &self.alert_id)?;
        }
        if self.status != 0 {
            let v = AlertStatus::try_from(self.status)
                .map_err(|_| serde::ser::Error::custom(format!("Invalid variant {}", self.status)))?;
            struct_ser.serialize_field("status", &v)?;
        }
        if !self.assignee.is_empty() {
            struct_ser.serialize_field("assignee", &self.assignee)?;
        }
        if !self.comment.is_empty() {
            struct_ser.serialize_field("comment", &self.comment)?;
        }
        if !self.updated_by.is_empty() {
            struct_ser.serialize_field("updatedBy", &self.updated_by)?;
        }
        if self.updated_at != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("updatedAt", ToString::to_string(&self.updated_at).as_str())?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for AlertUpdate {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "alert_id",
            "alertId",
            "status",
            "assignee",
            "comment",
            "updated_by",
            "updatedBy",
            "updated_at",
            "updatedAt",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            AlertId,
            Status,
            Assignee,
            Comment,
            UpdatedBy,
            UpdatedAt,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "alertId" | "alert_id" => Ok(GeneratedField::AlertId),
                            "status" => Ok(GeneratedField::Status),
                            "assignee" => Ok(GeneratedField::Assignee),
                            "comment" => Ok(GeneratedField::Comment),
                            "updatedBy" | "updated_by" => Ok(GeneratedField::UpdatedBy),
                            "updatedAt" | "updated_at" => Ok(GeneratedField::UpdatedAt),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = AlertUpdate;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.AlertUpdate")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<AlertUpdate, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut alert_id__ = None;
                let mut status__ = None;
                let mut assignee__ = None;
                let mut comment__ = None;
                let mut updated_by__ = None;
                let mut updated_at__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::AlertId => {
                            if alert_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("alertId"));
                            }
                            alert_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Status => {
                            if status__.is_some() {
                                return Err(serde::de::Error::duplicate_field("status"));
                            }
                            status__ = Some(map_.next_value::<AlertStatus>()? as i32);
                        }
                        GeneratedField::Assignee => {
                            if assignee__.is_some() {
                                return Err(serde::de::Error::duplicate_field("assignee"));
                            }
                            assignee__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Comment => {
                            if comment__.is_some() {
                                return Err(serde::de::Error::duplicate_field("comment"));
                            }
                            comment__ = Some(map_.next_value()?);
                        }
                        GeneratedField::UpdatedBy => {
                            if updated_by__.is_some() {
                                return Err(serde::de::Error::duplicate_field("updatedBy"));
                            }
                            updated_by__ = Some(map_.next_value()?);
                        }
                        GeneratedField::UpdatedAt => {
                            if updated_at__.is_some() {
                                return Err(serde::de::Error::duplicate_field("updatedAt"));
                            }
                            updated_at__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                    }
                }
                Ok(AlertUpdate {
                    alert_id: alert_id__.unwrap_or_default(),
                    status: status__.unwrap_or_default(),
                    assignee: assignee__.unwrap_or_default(),
                    comment: comment__.unwrap_or_default(),
                    updated_by: updated_by__.unwrap_or_default(),
                    updated_at: updated_at__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.AlertUpdate", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for Asset {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.asset_id.is_empty() {
            len += 1;
        }
        if !self.tenant_id.is_empty() {
            len += 1;
        }
        if !self.ip_address.is_empty() {
            len += 1;
        }
        if !self.mac_address.is_empty() {
            len += 1;
        }
        if !self.hostname.is_empty() {
            len += 1;
        }
        if !self.vendor.is_empty() {
            len += 1;
        }
        if !self.os_type.is_empty() {
            len += 1;
        }
        if !self.source.is_empty() {
            len += 1;
        }
        if self.first_seen != 0 {
            len += 1;
        }
        if self.last_seen != 0 {
            len += 1;
        }
        if !self.vlan_id.is_empty() {
            len += 1;
        }
        if !self.switch_port.is_empty() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.Asset", len)?;
        if !self.asset_id.is_empty() {
            struct_ser.serialize_field("assetId", &self.asset_id)?;
        }
        if !self.tenant_id.is_empty() {
            struct_ser.serialize_field("tenantId", &self.tenant_id)?;
        }
        if !self.ip_address.is_empty() {
            struct_ser.serialize_field("ipAddress", &self.ip_address)?;
        }
        if !self.mac_address.is_empty() {
            struct_ser.serialize_field("macAddress", &self.mac_address)?;
        }
        if !self.hostname.is_empty() {
            struct_ser.serialize_field("hostname", &self.hostname)?;
        }
        if !self.vendor.is_empty() {
            struct_ser.serialize_field("vendor", &self.vendor)?;
        }
        if !self.os_type.is_empty() {
            struct_ser.serialize_field("osType", &self.os_type)?;
        }
        if !self.source.is_empty() {
            struct_ser.serialize_field("source", &self.source)?;
        }
        if self.first_seen != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("firstSeen", ToString::to_string(&self.first_seen).as_str())?;
        }
        if self.last_seen != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("lastSeen", ToString::to_string(&self.last_seen).as_str())?;
        }
        if !self.vlan_id.is_empty() {
            struct_ser.serialize_field("vlanId", &self.vlan_id)?;
        }
        if !self.switch_port.is_empty() {
            struct_ser.serialize_field("switchPort", &self.switch_port)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for Asset {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "asset_id",
            "assetId",
            "tenant_id",
            "tenantId",
            "ip_address",
            "ipAddress",
            "mac_address",
            "macAddress",
            "hostname",
            "vendor",
            "os_type",
            "osType",
            "source",
            "first_seen",
            "firstSeen",
            "last_seen",
            "lastSeen",
            "vlan_id",
            "vlanId",
            "switch_port",
            "switchPort",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            AssetId,
            TenantId,
            IpAddress,
            MacAddress,
            Hostname,
            Vendor,
            OsType,
            Source,
            FirstSeen,
            LastSeen,
            VlanId,
            SwitchPort,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "assetId" | "asset_id" => Ok(GeneratedField::AssetId),
                            "tenantId" | "tenant_id" => Ok(GeneratedField::TenantId),
                            "ipAddress" | "ip_address" => Ok(GeneratedField::IpAddress),
                            "macAddress" | "mac_address" => Ok(GeneratedField::MacAddress),
                            "hostname" => Ok(GeneratedField::Hostname),
                            "vendor" => Ok(GeneratedField::Vendor),
                            "osType" | "os_type" => Ok(GeneratedField::OsType),
                            "source" => Ok(GeneratedField::Source),
                            "firstSeen" | "first_seen" => Ok(GeneratedField::FirstSeen),
                            "lastSeen" | "last_seen" => Ok(GeneratedField::LastSeen),
                            "vlanId" | "vlan_id" => Ok(GeneratedField::VlanId),
                            "switchPort" | "switch_port" => Ok(GeneratedField::SwitchPort),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = Asset;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.Asset")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<Asset, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut asset_id__ = None;
                let mut tenant_id__ = None;
                let mut ip_address__ = None;
                let mut mac_address__ = None;
                let mut hostname__ = None;
                let mut vendor__ = None;
                let mut os_type__ = None;
                let mut source__ = None;
                let mut first_seen__ = None;
                let mut last_seen__ = None;
                let mut vlan_id__ = None;
                let mut switch_port__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::AssetId => {
                            if asset_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("assetId"));
                            }
                            asset_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::TenantId => {
                            if tenant_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("tenantId"));
                            }
                            tenant_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::IpAddress => {
                            if ip_address__.is_some() {
                                return Err(serde::de::Error::duplicate_field("ipAddress"));
                            }
                            ip_address__ = Some(map_.next_value()?);
                        }
                        GeneratedField::MacAddress => {
                            if mac_address__.is_some() {
                                return Err(serde::de::Error::duplicate_field("macAddress"));
                            }
                            mac_address__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Hostname => {
                            if hostname__.is_some() {
                                return Err(serde::de::Error::duplicate_field("hostname"));
                            }
                            hostname__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Vendor => {
                            if vendor__.is_some() {
                                return Err(serde::de::Error::duplicate_field("vendor"));
                            }
                            vendor__ = Some(map_.next_value()?);
                        }
                        GeneratedField::OsType => {
                            if os_type__.is_some() {
                                return Err(serde::de::Error::duplicate_field("osType"));
                            }
                            os_type__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Source => {
                            if source__.is_some() {
                                return Err(serde::de::Error::duplicate_field("source"));
                            }
                            source__ = Some(map_.next_value()?);
                        }
                        GeneratedField::FirstSeen => {
                            if first_seen__.is_some() {
                                return Err(serde::de::Error::duplicate_field("firstSeen"));
                            }
                            first_seen__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::LastSeen => {
                            if last_seen__.is_some() {
                                return Err(serde::de::Error::duplicate_field("lastSeen"));
                            }
                            last_seen__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::VlanId => {
                            if vlan_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("vlanId"));
                            }
                            vlan_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::SwitchPort => {
                            if switch_port__.is_some() {
                                return Err(serde::de::Error::duplicate_field("switchPort"));
                            }
                            switch_port__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(Asset {
                    asset_id: asset_id__.unwrap_or_default(),
                    tenant_id: tenant_id__.unwrap_or_default(),
                    ip_address: ip_address__.unwrap_or_default(),
                    mac_address: mac_address__.unwrap_or_default(),
                    hostname: hostname__.unwrap_or_default(),
                    vendor: vendor__.unwrap_or_default(),
                    os_type: os_type__.unwrap_or_default(),
                    source: source__.unwrap_or_default(),
                    first_seen: first_seen__.unwrap_or_default(),
                    last_seen: last_seen__.unwrap_or_default(),
                    vlan_id: vlan_id__.unwrap_or_default(),
                    switch_port: switch_port__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.Asset", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for AssetEvent {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.event_id.is_empty() {
            len += 1;
        }
        if !self.asset_id.is_empty() {
            len += 1;
        }
        if !self.tenant_id.is_empty() {
            len += 1;
        }
        if !self.event_type.is_empty() {
            len += 1;
        }
        if !self.old_value.is_empty() {
            len += 1;
        }
        if !self.new_value.is_empty() {
            len += 1;
        }
        if self.created_at != 0 {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.AssetEvent", len)?;
        if !self.event_id.is_empty() {
            struct_ser.serialize_field("eventId", &self.event_id)?;
        }
        if !self.asset_id.is_empty() {
            struct_ser.serialize_field("assetId", &self.asset_id)?;
        }
        if !self.tenant_id.is_empty() {
            struct_ser.serialize_field("tenantId", &self.tenant_id)?;
        }
        if !self.event_type.is_empty() {
            struct_ser.serialize_field("eventType", &self.event_type)?;
        }
        if !self.old_value.is_empty() {
            struct_ser.serialize_field("oldValue", &self.old_value)?;
        }
        if !self.new_value.is_empty() {
            struct_ser.serialize_field("newValue", &self.new_value)?;
        }
        if self.created_at != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("createdAt", ToString::to_string(&self.created_at).as_str())?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for AssetEvent {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "event_id",
            "eventId",
            "asset_id",
            "assetId",
            "tenant_id",
            "tenantId",
            "event_type",
            "eventType",
            "old_value",
            "oldValue",
            "new_value",
            "newValue",
            "created_at",
            "createdAt",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            EventId,
            AssetId,
            TenantId,
            EventType,
            OldValue,
            NewValue,
            CreatedAt,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "eventId" | "event_id" => Ok(GeneratedField::EventId),
                            "assetId" | "asset_id" => Ok(GeneratedField::AssetId),
                            "tenantId" | "tenant_id" => Ok(GeneratedField::TenantId),
                            "eventType" | "event_type" => Ok(GeneratedField::EventType),
                            "oldValue" | "old_value" => Ok(GeneratedField::OldValue),
                            "newValue" | "new_value" => Ok(GeneratedField::NewValue),
                            "createdAt" | "created_at" => Ok(GeneratedField::CreatedAt),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = AssetEvent;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.AssetEvent")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<AssetEvent, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut event_id__ = None;
                let mut asset_id__ = None;
                let mut tenant_id__ = None;
                let mut event_type__ = None;
                let mut old_value__ = None;
                let mut new_value__ = None;
                let mut created_at__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::EventId => {
                            if event_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("eventId"));
                            }
                            event_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::AssetId => {
                            if asset_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("assetId"));
                            }
                            asset_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::TenantId => {
                            if tenant_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("tenantId"));
                            }
                            tenant_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::EventType => {
                            if event_type__.is_some() {
                                return Err(serde::de::Error::duplicate_field("eventType"));
                            }
                            event_type__ = Some(map_.next_value()?);
                        }
                        GeneratedField::OldValue => {
                            if old_value__.is_some() {
                                return Err(serde::de::Error::duplicate_field("oldValue"));
                            }
                            old_value__ = Some(map_.next_value()?);
                        }
                        GeneratedField::NewValue => {
                            if new_value__.is_some() {
                                return Err(serde::de::Error::duplicate_field("newValue"));
                            }
                            new_value__ = Some(map_.next_value()?);
                        }
                        GeneratedField::CreatedAt => {
                            if created_at__.is_some() {
                                return Err(serde::de::Error::duplicate_field("createdAt"));
                            }
                            created_at__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                    }
                }
                Ok(AssetEvent {
                    event_id: event_id__.unwrap_or_default(),
                    asset_id: asset_id__.unwrap_or_default(),
                    tenant_id: tenant_id__.unwrap_or_default(),
                    event_type: event_type__.unwrap_or_default(),
                    old_value: old_value__.unwrap_or_default(),
                    new_value: new_value__.unwrap_or_default(),
                    created_at: created_at__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.AssetEvent", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for AuditLog {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.event_id.is_empty() {
            len += 1;
        }
        if !self.tenant_id.is_empty() {
            len += 1;
        }
        if !self.user_id.is_empty() {
            len += 1;
        }
        if !self.action.is_empty() {
            len += 1;
        }
        if !self.object_type.is_empty() {
            len += 1;
        }
        if !self.object_id.is_empty() {
            len += 1;
        }
        if !self.detail.is_empty() {
            len += 1;
        }
        if !self.ip_addr.is_empty() {
            len += 1;
        }
        if !self.user_agent.is_empty() {
            len += 1;
        }
        if self.created_at != 0 {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.AuditLog", len)?;
        if !self.event_id.is_empty() {
            struct_ser.serialize_field("eventId", &self.event_id)?;
        }
        if !self.tenant_id.is_empty() {
            struct_ser.serialize_field("tenantId", &self.tenant_id)?;
        }
        if !self.user_id.is_empty() {
            struct_ser.serialize_field("userId", &self.user_id)?;
        }
        if !self.action.is_empty() {
            struct_ser.serialize_field("action", &self.action)?;
        }
        if !self.object_type.is_empty() {
            struct_ser.serialize_field("objectType", &self.object_type)?;
        }
        if !self.object_id.is_empty() {
            struct_ser.serialize_field("objectId", &self.object_id)?;
        }
        if !self.detail.is_empty() {
            struct_ser.serialize_field("detail", &self.detail)?;
        }
        if !self.ip_addr.is_empty() {
            struct_ser.serialize_field("ipAddr", &self.ip_addr)?;
        }
        if !self.user_agent.is_empty() {
            struct_ser.serialize_field("userAgent", &self.user_agent)?;
        }
        if self.created_at != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("createdAt", ToString::to_string(&self.created_at).as_str())?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for AuditLog {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "event_id",
            "eventId",
            "tenant_id",
            "tenantId",
            "user_id",
            "userId",
            "action",
            "object_type",
            "objectType",
            "object_id",
            "objectId",
            "detail",
            "ip_addr",
            "ipAddr",
            "user_agent",
            "userAgent",
            "created_at",
            "createdAt",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            EventId,
            TenantId,
            UserId,
            Action,
            ObjectType,
            ObjectId,
            Detail,
            IpAddr,
            UserAgent,
            CreatedAt,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "eventId" | "event_id" => Ok(GeneratedField::EventId),
                            "tenantId" | "tenant_id" => Ok(GeneratedField::TenantId),
                            "userId" | "user_id" => Ok(GeneratedField::UserId),
                            "action" => Ok(GeneratedField::Action),
                            "objectType" | "object_type" => Ok(GeneratedField::ObjectType),
                            "objectId" | "object_id" => Ok(GeneratedField::ObjectId),
                            "detail" => Ok(GeneratedField::Detail),
                            "ipAddr" | "ip_addr" => Ok(GeneratedField::IpAddr),
                            "userAgent" | "user_agent" => Ok(GeneratedField::UserAgent),
                            "createdAt" | "created_at" => Ok(GeneratedField::CreatedAt),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = AuditLog;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.AuditLog")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<AuditLog, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut event_id__ = None;
                let mut tenant_id__ = None;
                let mut user_id__ = None;
                let mut action__ = None;
                let mut object_type__ = None;
                let mut object_id__ = None;
                let mut detail__ = None;
                let mut ip_addr__ = None;
                let mut user_agent__ = None;
                let mut created_at__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::EventId => {
                            if event_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("eventId"));
                            }
                            event_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::TenantId => {
                            if tenant_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("tenantId"));
                            }
                            tenant_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::UserId => {
                            if user_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("userId"));
                            }
                            user_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Action => {
                            if action__.is_some() {
                                return Err(serde::de::Error::duplicate_field("action"));
                            }
                            action__ = Some(map_.next_value()?);
                        }
                        GeneratedField::ObjectType => {
                            if object_type__.is_some() {
                                return Err(serde::de::Error::duplicate_field("objectType"));
                            }
                            object_type__ = Some(map_.next_value()?);
                        }
                        GeneratedField::ObjectId => {
                            if object_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("objectId"));
                            }
                            object_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Detail => {
                            if detail__.is_some() {
                                return Err(serde::de::Error::duplicate_field("detail"));
                            }
                            detail__ = Some(map_.next_value()?);
                        }
                        GeneratedField::IpAddr => {
                            if ip_addr__.is_some() {
                                return Err(serde::de::Error::duplicate_field("ipAddr"));
                            }
                            ip_addr__ = Some(map_.next_value()?);
                        }
                        GeneratedField::UserAgent => {
                            if user_agent__.is_some() {
                                return Err(serde::de::Error::duplicate_field("userAgent"));
                            }
                            user_agent__ = Some(map_.next_value()?);
                        }
                        GeneratedField::CreatedAt => {
                            if created_at__.is_some() {
                                return Err(serde::de::Error::duplicate_field("createdAt"));
                            }
                            created_at__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                    }
                }
                Ok(AuditLog {
                    event_id: event_id__.unwrap_or_default(),
                    tenant_id: tenant_id__.unwrap_or_default(),
                    user_id: user_id__.unwrap_or_default(),
                    action: action__.unwrap_or_default(),
                    object_type: object_type__.unwrap_or_default(),
                    object_id: object_id__.unwrap_or_default(),
                    detail: detail__.unwrap_or_default(),
                    ip_addr: ip_addr__.unwrap_or_default(),
                    user_agent: user_agent__.unwrap_or_default(),
                    created_at: created_at__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.AuditLog", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for AuditLogBatch {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.events.is_empty() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.AuditLogBatch", len)?;
        if !self.events.is_empty() {
            struct_ser.serialize_field("events", &self.events)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for AuditLogBatch {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "events",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Events,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "events" => Ok(GeneratedField::Events),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = AuditLogBatch;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.AuditLogBatch")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<AuditLogBatch, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut events__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Events => {
                            if events__.is_some() {
                                return Err(serde::de::Error::duplicate_field("events"));
                            }
                            events__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(AuditLogBatch {
                    events: events__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.AuditLogBatch", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for BatchMetadata {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.batch_id.is_empty() {
            len += 1;
        }
        if !self.tenant_id.is_empty() {
            len += 1;
        }
        if !self.probe_id.is_empty() {
            len += 1;
        }
        if !self.run_id.is_empty() {
            len += 1;
        }
        if self.batch_size != 0 {
            len += 1;
        }
        if !self.compression.is_empty() {
            len += 1;
        }
        if self.created_at != 0 {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.BatchMetadata", len)?;
        if !self.batch_id.is_empty() {
            struct_ser.serialize_field("batchId", &self.batch_id)?;
        }
        if !self.tenant_id.is_empty() {
            struct_ser.serialize_field("tenantId", &self.tenant_id)?;
        }
        if !self.probe_id.is_empty() {
            struct_ser.serialize_field("probeId", &self.probe_id)?;
        }
        if !self.run_id.is_empty() {
            struct_ser.serialize_field("runId", &self.run_id)?;
        }
        if self.batch_size != 0 {
            struct_ser.serialize_field("batchSize", &self.batch_size)?;
        }
        if !self.compression.is_empty() {
            struct_ser.serialize_field("compression", &self.compression)?;
        }
        if self.created_at != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("createdAt", ToString::to_string(&self.created_at).as_str())?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for BatchMetadata {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "batch_id",
            "batchId",
            "tenant_id",
            "tenantId",
            "probe_id",
            "probeId",
            "run_id",
            "runId",
            "batch_size",
            "batchSize",
            "compression",
            "created_at",
            "createdAt",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            BatchId,
            TenantId,
            ProbeId,
            RunId,
            BatchSize,
            Compression,
            CreatedAt,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "batchId" | "batch_id" => Ok(GeneratedField::BatchId),
                            "tenantId" | "tenant_id" => Ok(GeneratedField::TenantId),
                            "probeId" | "probe_id" => Ok(GeneratedField::ProbeId),
                            "runId" | "run_id" => Ok(GeneratedField::RunId),
                            "batchSize" | "batch_size" => Ok(GeneratedField::BatchSize),
                            "compression" => Ok(GeneratedField::Compression),
                            "createdAt" | "created_at" => Ok(GeneratedField::CreatedAt),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = BatchMetadata;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.BatchMetadata")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<BatchMetadata, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut batch_id__ = None;
                let mut tenant_id__ = None;
                let mut probe_id__ = None;
                let mut run_id__ = None;
                let mut batch_size__ = None;
                let mut compression__ = None;
                let mut created_at__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::BatchId => {
                            if batch_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("batchId"));
                            }
                            batch_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::TenantId => {
                            if tenant_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("tenantId"));
                            }
                            tenant_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::ProbeId => {
                            if probe_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("probeId"));
                            }
                            probe_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::RunId => {
                            if run_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("runId"));
                            }
                            run_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::BatchSize => {
                            if batch_size__.is_some() {
                                return Err(serde::de::Error::duplicate_field("batchSize"));
                            }
                            batch_size__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::Compression => {
                            if compression__.is_some() {
                                return Err(serde::de::Error::duplicate_field("compression"));
                            }
                            compression__ = Some(map_.next_value()?);
                        }
                        GeneratedField::CreatedAt => {
                            if created_at__.is_some() {
                                return Err(serde::de::Error::duplicate_field("createdAt"));
                            }
                            created_at__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                    }
                }
                Ok(BatchMetadata {
                    batch_id: batch_id__.unwrap_or_default(),
                    tenant_id: tenant_id__.unwrap_or_default(),
                    probe_id: probe_id__.unwrap_or_default(),
                    run_id: run_id__.unwrap_or_default(),
                    batch_size: batch_size__.unwrap_or_default(),
                    compression: compression__.unwrap_or_default(),
                    created_at: created_at__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.BatchMetadata", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for CpuAffinityConfig {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.cpu_cores.is_empty() {
            len += 1;
        }
        if self.numa_aware {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.CPUAffinityConfig", len)?;
        if !self.cpu_cores.is_empty() {
            struct_ser.serialize_field("cpuCores", &self.cpu_cores)?;
        }
        if self.numa_aware {
            struct_ser.serialize_field("numaAware", &self.numa_aware)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for CpuAffinityConfig {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "cpu_cores",
            "cpuCores",
            "numa_aware",
            "numaAware",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            CpuCores,
            NumaAware,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "cpuCores" | "cpu_cores" => Ok(GeneratedField::CpuCores),
                            "numaAware" | "numa_aware" => Ok(GeneratedField::NumaAware),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = CpuAffinityConfig;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.CPUAffinityConfig")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<CpuAffinityConfig, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut cpu_cores__ = None;
                let mut numa_aware__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::CpuCores => {
                            if cpu_cores__.is_some() {
                                return Err(serde::de::Error::duplicate_field("cpuCores"));
                            }
                            cpu_cores__ = 
                                Some(map_.next_value::<Vec<::pbjson::private::NumberDeserialize<_>>>()?
                                    .into_iter().map(|x| x.0).collect())
                            ;
                        }
                        GeneratedField::NumaAware => {
                            if numa_aware__.is_some() {
                                return Err(serde::de::Error::duplicate_field("numaAware"));
                            }
                            numa_aware__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(CpuAffinityConfig {
                    cpu_cores: cpu_cores__.unwrap_or_default(),
                    numa_aware: numa_aware__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.CPUAffinityConfig", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for Campaign {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.tenant_id.is_empty() {
            len += 1;
        }
        if !self.campaign_id.is_empty() {
            len += 1;
        }
        if self.ts_start != 0 {
            len += 1;
        }
        if self.ts_end != 0 {
            len += 1;
        }
        if !self.entities.is_empty() {
            len += 1;
        }
        if !self.alerts.is_empty() {
            len += 1;
        }
        if self.score != 0. {
            len += 1;
        }
        if !self.summary.is_empty() {
            len += 1;
        }
        if !self.event_id.is_empty() {
            len += 1;
        }
        if self.ingest_ts != 0 {
            len += 1;
        }
        if self.header.is_some() {
            len += 1;
        }
        if !self.campaign_type.is_empty() {
            len += 1;
        }
        if !self.attack_phases.is_empty() {
            len += 1;
        }
        if !self.rule_ids.is_empty() {
            len += 1;
        }
        if !self.model_ids.is_empty() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.Campaign", len)?;
        if !self.tenant_id.is_empty() {
            struct_ser.serialize_field("tenantId", &self.tenant_id)?;
        }
        if !self.campaign_id.is_empty() {
            struct_ser.serialize_field("campaignId", &self.campaign_id)?;
        }
        if self.ts_start != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("tsStart", ToString::to_string(&self.ts_start).as_str())?;
        }
        if self.ts_end != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("tsEnd", ToString::to_string(&self.ts_end).as_str())?;
        }
        if !self.entities.is_empty() {
            struct_ser.serialize_field("entities", &self.entities)?;
        }
        if !self.alerts.is_empty() {
            struct_ser.serialize_field("alerts", &self.alerts)?;
        }
        if self.score != 0. {
            struct_ser.serialize_field("score", &self.score)?;
        }
        if !self.summary.is_empty() {
            struct_ser.serialize_field("summary", &self.summary)?;
        }
        if !self.event_id.is_empty() {
            struct_ser.serialize_field("eventId", &self.event_id)?;
        }
        if self.ingest_ts != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("ingestTs", ToString::to_string(&self.ingest_ts).as_str())?;
        }
        if let Some(v) = self.header.as_ref() {
            struct_ser.serialize_field("header", v)?;
        }
        if !self.campaign_type.is_empty() {
            struct_ser.serialize_field("campaignType", &self.campaign_type)?;
        }
        if !self.attack_phases.is_empty() {
            struct_ser.serialize_field("attackPhases", &self.attack_phases)?;
        }
        if !self.rule_ids.is_empty() {
            struct_ser.serialize_field("ruleIds", &self.rule_ids)?;
        }
        if !self.model_ids.is_empty() {
            struct_ser.serialize_field("modelIds", &self.model_ids)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for Campaign {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "tenant_id",
            "tenantId",
            "campaign_id",
            "campaignId",
            "ts_start",
            "tsStart",
            "ts_end",
            "tsEnd",
            "entities",
            "alerts",
            "score",
            "summary",
            "event_id",
            "eventId",
            "ingest_ts",
            "ingestTs",
            "header",
            "campaign_type",
            "campaignType",
            "attack_phases",
            "attackPhases",
            "rule_ids",
            "ruleIds",
            "model_ids",
            "modelIds",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            TenantId,
            CampaignId,
            TsStart,
            TsEnd,
            Entities,
            Alerts,
            Score,
            Summary,
            EventId,
            IngestTs,
            Header,
            CampaignType,
            AttackPhases,
            RuleIds,
            ModelIds,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "tenantId" | "tenant_id" => Ok(GeneratedField::TenantId),
                            "campaignId" | "campaign_id" => Ok(GeneratedField::CampaignId),
                            "tsStart" | "ts_start" => Ok(GeneratedField::TsStart),
                            "tsEnd" | "ts_end" => Ok(GeneratedField::TsEnd),
                            "entities" => Ok(GeneratedField::Entities),
                            "alerts" => Ok(GeneratedField::Alerts),
                            "score" => Ok(GeneratedField::Score),
                            "summary" => Ok(GeneratedField::Summary),
                            "eventId" | "event_id" => Ok(GeneratedField::EventId),
                            "ingestTs" | "ingest_ts" => Ok(GeneratedField::IngestTs),
                            "header" => Ok(GeneratedField::Header),
                            "campaignType" | "campaign_type" => Ok(GeneratedField::CampaignType),
                            "attackPhases" | "attack_phases" => Ok(GeneratedField::AttackPhases),
                            "ruleIds" | "rule_ids" => Ok(GeneratedField::RuleIds),
                            "modelIds" | "model_ids" => Ok(GeneratedField::ModelIds),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = Campaign;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.Campaign")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<Campaign, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut tenant_id__ = None;
                let mut campaign_id__ = None;
                let mut ts_start__ = None;
                let mut ts_end__ = None;
                let mut entities__ = None;
                let mut alerts__ = None;
                let mut score__ = None;
                let mut summary__ = None;
                let mut event_id__ = None;
                let mut ingest_ts__ = None;
                let mut header__ = None;
                let mut campaign_type__ = None;
                let mut attack_phases__ = None;
                let mut rule_ids__ = None;
                let mut model_ids__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::TenantId => {
                            if tenant_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("tenantId"));
                            }
                            tenant_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::CampaignId => {
                            if campaign_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("campaignId"));
                            }
                            campaign_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::TsStart => {
                            if ts_start__.is_some() {
                                return Err(serde::de::Error::duplicate_field("tsStart"));
                            }
                            ts_start__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::TsEnd => {
                            if ts_end__.is_some() {
                                return Err(serde::de::Error::duplicate_field("tsEnd"));
                            }
                            ts_end__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::Entities => {
                            if entities__.is_some() {
                                return Err(serde::de::Error::duplicate_field("entities"));
                            }
                            entities__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Alerts => {
                            if alerts__.is_some() {
                                return Err(serde::de::Error::duplicate_field("alerts"));
                            }
                            alerts__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Score => {
                            if score__.is_some() {
                                return Err(serde::de::Error::duplicate_field("score"));
                            }
                            score__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::Summary => {
                            if summary__.is_some() {
                                return Err(serde::de::Error::duplicate_field("summary"));
                            }
                            summary__ = Some(map_.next_value()?);
                        }
                        GeneratedField::EventId => {
                            if event_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("eventId"));
                            }
                            event_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::IngestTs => {
                            if ingest_ts__.is_some() {
                                return Err(serde::de::Error::duplicate_field("ingestTs"));
                            }
                            ingest_ts__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::Header => {
                            if header__.is_some() {
                                return Err(serde::de::Error::duplicate_field("header"));
                            }
                            header__ = map_.next_value()?;
                        }
                        GeneratedField::CampaignType => {
                            if campaign_type__.is_some() {
                                return Err(serde::de::Error::duplicate_field("campaignType"));
                            }
                            campaign_type__ = Some(map_.next_value()?);
                        }
                        GeneratedField::AttackPhases => {
                            if attack_phases__.is_some() {
                                return Err(serde::de::Error::duplicate_field("attackPhases"));
                            }
                            attack_phases__ = Some(map_.next_value()?);
                        }
                        GeneratedField::RuleIds => {
                            if rule_ids__.is_some() {
                                return Err(serde::de::Error::duplicate_field("ruleIds"));
                            }
                            rule_ids__ = Some(map_.next_value()?);
                        }
                        GeneratedField::ModelIds => {
                            if model_ids__.is_some() {
                                return Err(serde::de::Error::duplicate_field("modelIds"));
                            }
                            model_ids__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(Campaign {
                    tenant_id: tenant_id__.unwrap_or_default(),
                    campaign_id: campaign_id__.unwrap_or_default(),
                    ts_start: ts_start__.unwrap_or_default(),
                    ts_end: ts_end__.unwrap_or_default(),
                    entities: entities__.unwrap_or_default(),
                    alerts: alerts__.unwrap_or_default(),
                    score: score__.unwrap_or_default(),
                    summary: summary__.unwrap_or_default(),
                    event_id: event_id__.unwrap_or_default(),
                    ingest_ts: ingest_ts__.unwrap_or_default(),
                    header: header__,
                    campaign_type: campaign_type__.unwrap_or_default(),
                    attack_phases: attack_phases__.unwrap_or_default(),
                    rule_ids: rule_ids__.unwrap_or_default(),
                    model_ids: model_ids__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.Campaign", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for CampaignBatch {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.campaigns.is_empty() {
            len += 1;
        }
        if !self.batch_id.is_empty() {
            len += 1;
        }
        if !self.tenant_id.is_empty() {
            len += 1;
        }
        if self.created_at != 0 {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.CampaignBatch", len)?;
        if !self.campaigns.is_empty() {
            struct_ser.serialize_field("campaigns", &self.campaigns)?;
        }
        if !self.batch_id.is_empty() {
            struct_ser.serialize_field("batchId", &self.batch_id)?;
        }
        if !self.tenant_id.is_empty() {
            struct_ser.serialize_field("tenantId", &self.tenant_id)?;
        }
        if self.created_at != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("createdAt", ToString::to_string(&self.created_at).as_str())?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for CampaignBatch {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "campaigns",
            "batch_id",
            "batchId",
            "tenant_id",
            "tenantId",
            "created_at",
            "createdAt",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Campaigns,
            BatchId,
            TenantId,
            CreatedAt,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "campaigns" => Ok(GeneratedField::Campaigns),
                            "batchId" | "batch_id" => Ok(GeneratedField::BatchId),
                            "tenantId" | "tenant_id" => Ok(GeneratedField::TenantId),
                            "createdAt" | "created_at" => Ok(GeneratedField::CreatedAt),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = CampaignBatch;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.CampaignBatch")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<CampaignBatch, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut campaigns__ = None;
                let mut batch_id__ = None;
                let mut tenant_id__ = None;
                let mut created_at__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Campaigns => {
                            if campaigns__.is_some() {
                                return Err(serde::de::Error::duplicate_field("campaigns"));
                            }
                            campaigns__ = Some(map_.next_value()?);
                        }
                        GeneratedField::BatchId => {
                            if batch_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("batchId"));
                            }
                            batch_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::TenantId => {
                            if tenant_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("tenantId"));
                            }
                            tenant_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::CreatedAt => {
                            if created_at__.is_some() {
                                return Err(serde::de::Error::duplicate_field("createdAt"));
                            }
                            created_at__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                    }
                }
                Ok(CampaignBatch {
                    campaigns: campaigns__.unwrap_or_default(),
                    batch_id: batch_id__.unwrap_or_default(),
                    tenant_id: tenant_id__.unwrap_or_default(),
                    created_at: created_at__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.CampaignBatch", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for CampaignQuery {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.tenant_id.is_empty() {
            len += 1;
        }
        if !self.campaign_id.is_empty() {
            len += 1;
        }
        if self.start_time != 0 {
            len += 1;
        }
        if self.end_time != 0 {
            len += 1;
        }
        if !self.campaign_types.is_empty() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.CampaignQuery", len)?;
        if !self.tenant_id.is_empty() {
            struct_ser.serialize_field("tenantId", &self.tenant_id)?;
        }
        if !self.campaign_id.is_empty() {
            struct_ser.serialize_field("campaignId", &self.campaign_id)?;
        }
        if self.start_time != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("startTime", ToString::to_string(&self.start_time).as_str())?;
        }
        if self.end_time != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("endTime", ToString::to_string(&self.end_time).as_str())?;
        }
        if !self.campaign_types.is_empty() {
            struct_ser.serialize_field("campaignTypes", &self.campaign_types)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for CampaignQuery {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "tenant_id",
            "tenantId",
            "campaign_id",
            "campaignId",
            "start_time",
            "startTime",
            "end_time",
            "endTime",
            "campaign_types",
            "campaignTypes",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            TenantId,
            CampaignId,
            StartTime,
            EndTime,
            CampaignTypes,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "tenantId" | "tenant_id" => Ok(GeneratedField::TenantId),
                            "campaignId" | "campaign_id" => Ok(GeneratedField::CampaignId),
                            "startTime" | "start_time" => Ok(GeneratedField::StartTime),
                            "endTime" | "end_time" => Ok(GeneratedField::EndTime),
                            "campaignTypes" | "campaign_types" => Ok(GeneratedField::CampaignTypes),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = CampaignQuery;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.CampaignQuery")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<CampaignQuery, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut tenant_id__ = None;
                let mut campaign_id__ = None;
                let mut start_time__ = None;
                let mut end_time__ = None;
                let mut campaign_types__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::TenantId => {
                            if tenant_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("tenantId"));
                            }
                            tenant_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::CampaignId => {
                            if campaign_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("campaignId"));
                            }
                            campaign_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::StartTime => {
                            if start_time__.is_some() {
                                return Err(serde::de::Error::duplicate_field("startTime"));
                            }
                            start_time__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::EndTime => {
                            if end_time__.is_some() {
                                return Err(serde::de::Error::duplicate_field("endTime"));
                            }
                            end_time__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::CampaignTypes => {
                            if campaign_types__.is_some() {
                                return Err(serde::de::Error::duplicate_field("campaignTypes"));
                            }
                            campaign_types__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(CampaignQuery {
                    tenant_id: tenant_id__.unwrap_or_default(),
                    campaign_id: campaign_id__.unwrap_or_default(),
                    start_time: start_time__.unwrap_or_default(),
                    end_time: end_time__.unwrap_or_default(),
                    campaign_types: campaign_types__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.CampaignQuery", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for CampaignQueryResponse {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.campaigns.is_empty() {
            len += 1;
        }
        if self.total_count != 0 {
            len += 1;
        }
        if self.has_more {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.CampaignQueryResponse", len)?;
        if !self.campaigns.is_empty() {
            struct_ser.serialize_field("campaigns", &self.campaigns)?;
        }
        if self.total_count != 0 {
            struct_ser.serialize_field("totalCount", &self.total_count)?;
        }
        if self.has_more {
            struct_ser.serialize_field("hasMore", &self.has_more)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for CampaignQueryResponse {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "campaigns",
            "total_count",
            "totalCount",
            "has_more",
            "hasMore",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Campaigns,
            TotalCount,
            HasMore,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "campaigns" => Ok(GeneratedField::Campaigns),
                            "totalCount" | "total_count" => Ok(GeneratedField::TotalCount),
                            "hasMore" | "has_more" => Ok(GeneratedField::HasMore),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = CampaignQueryResponse;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.CampaignQueryResponse")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<CampaignQueryResponse, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut campaigns__ = None;
                let mut total_count__ = None;
                let mut has_more__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Campaigns => {
                            if campaigns__.is_some() {
                                return Err(serde::de::Error::duplicate_field("campaigns"));
                            }
                            campaigns__ = Some(map_.next_value()?);
                        }
                        GeneratedField::TotalCount => {
                            if total_count__.is_some() {
                                return Err(serde::de::Error::duplicate_field("totalCount"));
                            }
                            total_count__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::HasMore => {
                            if has_more__.is_some() {
                                return Err(serde::de::Error::duplicate_field("hasMore"));
                            }
                            has_more__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(CampaignQueryResponse {
                    campaigns: campaigns__.unwrap_or_default(),
                    total_count: total_count__.unwrap_or_default(),
                    has_more: has_more__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.CampaignQueryResponse", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for DeadLetter {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.event_id.is_empty() {
            len += 1;
        }
        if !self.tenant_id.is_empty() {
            len += 1;
        }
        if !self.source_topic.is_empty() {
            len += 1;
        }
        if !self.source_key.is_empty() {
            len += 1;
        }
        if !self.error_msg.is_empty() {
            len += 1;
        }
        if !self.raw_payload.is_empty() {
            len += 1;
        }
        if self.retry_count != 0 {
            len += 1;
        }
        if self.created_at != 0 {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.DeadLetter", len)?;
        if !self.event_id.is_empty() {
            struct_ser.serialize_field("eventId", &self.event_id)?;
        }
        if !self.tenant_id.is_empty() {
            struct_ser.serialize_field("tenantId", &self.tenant_id)?;
        }
        if !self.source_topic.is_empty() {
            struct_ser.serialize_field("sourceTopic", &self.source_topic)?;
        }
        if !self.source_key.is_empty() {
            struct_ser.serialize_field("sourceKey", &self.source_key)?;
        }
        if !self.error_msg.is_empty() {
            struct_ser.serialize_field("errorMsg", &self.error_msg)?;
        }
        if !self.raw_payload.is_empty() {
            struct_ser.serialize_field("rawPayload", &self.raw_payload)?;
        }
        if self.retry_count != 0 {
            struct_ser.serialize_field("retryCount", &self.retry_count)?;
        }
        if self.created_at != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("createdAt", ToString::to_string(&self.created_at).as_str())?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for DeadLetter {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "event_id",
            "eventId",
            "tenant_id",
            "tenantId",
            "source_topic",
            "sourceTopic",
            "source_key",
            "sourceKey",
            "error_msg",
            "errorMsg",
            "raw_payload",
            "rawPayload",
            "retry_count",
            "retryCount",
            "created_at",
            "createdAt",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            EventId,
            TenantId,
            SourceTopic,
            SourceKey,
            ErrorMsg,
            RawPayload,
            RetryCount,
            CreatedAt,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "eventId" | "event_id" => Ok(GeneratedField::EventId),
                            "tenantId" | "tenant_id" => Ok(GeneratedField::TenantId),
                            "sourceTopic" | "source_topic" => Ok(GeneratedField::SourceTopic),
                            "sourceKey" | "source_key" => Ok(GeneratedField::SourceKey),
                            "errorMsg" | "error_msg" => Ok(GeneratedField::ErrorMsg),
                            "rawPayload" | "raw_payload" => Ok(GeneratedField::RawPayload),
                            "retryCount" | "retry_count" => Ok(GeneratedField::RetryCount),
                            "createdAt" | "created_at" => Ok(GeneratedField::CreatedAt),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = DeadLetter;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.DeadLetter")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<DeadLetter, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut event_id__ = None;
                let mut tenant_id__ = None;
                let mut source_topic__ = None;
                let mut source_key__ = None;
                let mut error_msg__ = None;
                let mut raw_payload__ = None;
                let mut retry_count__ = None;
                let mut created_at__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::EventId => {
                            if event_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("eventId"));
                            }
                            event_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::TenantId => {
                            if tenant_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("tenantId"));
                            }
                            tenant_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::SourceTopic => {
                            if source_topic__.is_some() {
                                return Err(serde::de::Error::duplicate_field("sourceTopic"));
                            }
                            source_topic__ = Some(map_.next_value()?);
                        }
                        GeneratedField::SourceKey => {
                            if source_key__.is_some() {
                                return Err(serde::de::Error::duplicate_field("sourceKey"));
                            }
                            source_key__ = Some(map_.next_value()?);
                        }
                        GeneratedField::ErrorMsg => {
                            if error_msg__.is_some() {
                                return Err(serde::de::Error::duplicate_field("errorMsg"));
                            }
                            error_msg__ = Some(map_.next_value()?);
                        }
                        GeneratedField::RawPayload => {
                            if raw_payload__.is_some() {
                                return Err(serde::de::Error::duplicate_field("rawPayload"));
                            }
                            raw_payload__ = Some(map_.next_value()?);
                        }
                        GeneratedField::RetryCount => {
                            if retry_count__.is_some() {
                                return Err(serde::de::Error::duplicate_field("retryCount"));
                            }
                            retry_count__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::CreatedAt => {
                            if created_at__.is_some() {
                                return Err(serde::de::Error::duplicate_field("createdAt"));
                            }
                            created_at__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                    }
                }
                Ok(DeadLetter {
                    event_id: event_id__.unwrap_or_default(),
                    tenant_id: tenant_id__.unwrap_or_default(),
                    source_topic: source_topic__.unwrap_or_default(),
                    source_key: source_key__.unwrap_or_default(),
                    error_msg: error_msg__.unwrap_or_default(),
                    raw_payload: raw_payload__.unwrap_or_default(),
                    retry_count: retry_count__.unwrap_or_default(),
                    created_at: created_at__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.DeadLetter", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for DeadLetterBatch {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.events.is_empty() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.DeadLetterBatch", len)?;
        if !self.events.is_empty() {
            struct_ser.serialize_field("events", &self.events)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for DeadLetterBatch {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "events",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Events,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "events" => Ok(GeneratedField::Events),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = DeadLetterBatch;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.DeadLetterBatch")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<DeadLetterBatch, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut events__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Events => {
                            if events__.is_some() {
                                return Err(serde::de::Error::duplicate_field("events"));
                            }
                            events__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(DeadLetterBatch {
                    events: events__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.DeadLetterBatch", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for DedupStats {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.tenant_id.is_empty() {
            len += 1;
        }
        if !self.fingerprint.is_empty() {
            len += 1;
        }
        if !self.alert_type.is_empty() {
            len += 1;
        }
        if !self.severity.is_empty() {
            len += 1;
        }
        if !self.src_ip.is_empty() {
            len += 1;
        }
        if !self.dst_ip.is_empty() {
            len += 1;
        }
        if self.dst_port != 0 {
            len += 1;
        }
        if self.first_seen != 0 {
            len += 1;
        }
        if self.last_seen != 0 {
            len += 1;
        }
        if self.occurrence_count != 0 {
            len += 1;
        }
        if !self.sample_alert_ids.is_empty() {
            len += 1;
        }
        if self.ingest_ts != 0 {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.DedupStats", len)?;
        if !self.tenant_id.is_empty() {
            struct_ser.serialize_field("tenantId", &self.tenant_id)?;
        }
        if !self.fingerprint.is_empty() {
            struct_ser.serialize_field("fingerprint", &self.fingerprint)?;
        }
        if !self.alert_type.is_empty() {
            struct_ser.serialize_field("alertType", &self.alert_type)?;
        }
        if !self.severity.is_empty() {
            struct_ser.serialize_field("severity", &self.severity)?;
        }
        if !self.src_ip.is_empty() {
            struct_ser.serialize_field("srcIp", &self.src_ip)?;
        }
        if !self.dst_ip.is_empty() {
            struct_ser.serialize_field("dstIp", &self.dst_ip)?;
        }
        if self.dst_port != 0 {
            struct_ser.serialize_field("dstPort", &self.dst_port)?;
        }
        if self.first_seen != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("firstSeen", ToString::to_string(&self.first_seen).as_str())?;
        }
        if self.last_seen != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("lastSeen", ToString::to_string(&self.last_seen).as_str())?;
        }
        if self.occurrence_count != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("occurrenceCount", ToString::to_string(&self.occurrence_count).as_str())?;
        }
        if !self.sample_alert_ids.is_empty() {
            struct_ser.serialize_field("sampleAlertIds", &self.sample_alert_ids)?;
        }
        if self.ingest_ts != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("ingestTs", ToString::to_string(&self.ingest_ts).as_str())?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for DedupStats {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "tenant_id",
            "tenantId",
            "fingerprint",
            "alert_type",
            "alertType",
            "severity",
            "src_ip",
            "srcIp",
            "dst_ip",
            "dstIp",
            "dst_port",
            "dstPort",
            "first_seen",
            "firstSeen",
            "last_seen",
            "lastSeen",
            "occurrence_count",
            "occurrenceCount",
            "sample_alert_ids",
            "sampleAlertIds",
            "ingest_ts",
            "ingestTs",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            TenantId,
            Fingerprint,
            AlertType,
            Severity,
            SrcIp,
            DstIp,
            DstPort,
            FirstSeen,
            LastSeen,
            OccurrenceCount,
            SampleAlertIds,
            IngestTs,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "tenantId" | "tenant_id" => Ok(GeneratedField::TenantId),
                            "fingerprint" => Ok(GeneratedField::Fingerprint),
                            "alertType" | "alert_type" => Ok(GeneratedField::AlertType),
                            "severity" => Ok(GeneratedField::Severity),
                            "srcIp" | "src_ip" => Ok(GeneratedField::SrcIp),
                            "dstIp" | "dst_ip" => Ok(GeneratedField::DstIp),
                            "dstPort" | "dst_port" => Ok(GeneratedField::DstPort),
                            "firstSeen" | "first_seen" => Ok(GeneratedField::FirstSeen),
                            "lastSeen" | "last_seen" => Ok(GeneratedField::LastSeen),
                            "occurrenceCount" | "occurrence_count" => Ok(GeneratedField::OccurrenceCount),
                            "sampleAlertIds" | "sample_alert_ids" => Ok(GeneratedField::SampleAlertIds),
                            "ingestTs" | "ingest_ts" => Ok(GeneratedField::IngestTs),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = DedupStats;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.DedupStats")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<DedupStats, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut tenant_id__ = None;
                let mut fingerprint__ = None;
                let mut alert_type__ = None;
                let mut severity__ = None;
                let mut src_ip__ = None;
                let mut dst_ip__ = None;
                let mut dst_port__ = None;
                let mut first_seen__ = None;
                let mut last_seen__ = None;
                let mut occurrence_count__ = None;
                let mut sample_alert_ids__ = None;
                let mut ingest_ts__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::TenantId => {
                            if tenant_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("tenantId"));
                            }
                            tenant_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Fingerprint => {
                            if fingerprint__.is_some() {
                                return Err(serde::de::Error::duplicate_field("fingerprint"));
                            }
                            fingerprint__ = Some(map_.next_value()?);
                        }
                        GeneratedField::AlertType => {
                            if alert_type__.is_some() {
                                return Err(serde::de::Error::duplicate_field("alertType"));
                            }
                            alert_type__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Severity => {
                            if severity__.is_some() {
                                return Err(serde::de::Error::duplicate_field("severity"));
                            }
                            severity__ = Some(map_.next_value()?);
                        }
                        GeneratedField::SrcIp => {
                            if src_ip__.is_some() {
                                return Err(serde::de::Error::duplicate_field("srcIp"));
                            }
                            src_ip__ = Some(map_.next_value()?);
                        }
                        GeneratedField::DstIp => {
                            if dst_ip__.is_some() {
                                return Err(serde::de::Error::duplicate_field("dstIp"));
                            }
                            dst_ip__ = Some(map_.next_value()?);
                        }
                        GeneratedField::DstPort => {
                            if dst_port__.is_some() {
                                return Err(serde::de::Error::duplicate_field("dstPort"));
                            }
                            dst_port__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::FirstSeen => {
                            if first_seen__.is_some() {
                                return Err(serde::de::Error::duplicate_field("firstSeen"));
                            }
                            first_seen__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::LastSeen => {
                            if last_seen__.is_some() {
                                return Err(serde::de::Error::duplicate_field("lastSeen"));
                            }
                            last_seen__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::OccurrenceCount => {
                            if occurrence_count__.is_some() {
                                return Err(serde::de::Error::duplicate_field("occurrenceCount"));
                            }
                            occurrence_count__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::SampleAlertIds => {
                            if sample_alert_ids__.is_some() {
                                return Err(serde::de::Error::duplicate_field("sampleAlertIds"));
                            }
                            sample_alert_ids__ = Some(map_.next_value()?);
                        }
                        GeneratedField::IngestTs => {
                            if ingest_ts__.is_some() {
                                return Err(serde::de::Error::duplicate_field("ingestTs"));
                            }
                            ingest_ts__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                    }
                }
                Ok(DedupStats {
                    tenant_id: tenant_id__.unwrap_or_default(),
                    fingerprint: fingerprint__.unwrap_or_default(),
                    alert_type: alert_type__.unwrap_or_default(),
                    severity: severity__.unwrap_or_default(),
                    src_ip: src_ip__.unwrap_or_default(),
                    dst_ip: dst_ip__.unwrap_or_default(),
                    dst_port: dst_port__.unwrap_or_default(),
                    first_seen: first_seen__.unwrap_or_default(),
                    last_seen: last_seen__.unwrap_or_default(),
                    occurrence_count: occurrence_count__.unwrap_or_default(),
                    sample_alert_ids: sample_alert_ids__.unwrap_or_default(),
                    ingest_ts: ingest_ts__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.DedupStats", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for DeploymentStatus {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        let variant = match self {
            Self::Unspecified => "DEPLOYMENT_STATUS_UNSPECIFIED",
            Self::Planned => "DEPLOYMENT_STATUS_PLANNED",
            Self::Gray => "DEPLOYMENT_STATUS_GRAY",
            Self::Active => "DEPLOYMENT_STATUS_ACTIVE",
            Self::Paused => "DEPLOYMENT_STATUS_PAUSED",
            Self::RolledBack => "DEPLOYMENT_STATUS_ROLLED_BACK",
        };
        serializer.serialize_str(variant)
    }
}
impl<'de> serde::Deserialize<'de> for DeploymentStatus {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "DEPLOYMENT_STATUS_UNSPECIFIED",
            "DEPLOYMENT_STATUS_PLANNED",
            "DEPLOYMENT_STATUS_GRAY",
            "DEPLOYMENT_STATUS_ACTIVE",
            "DEPLOYMENT_STATUS_PAUSED",
            "DEPLOYMENT_STATUS_ROLLED_BACK",
        ];

        struct GeneratedVisitor;

        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = DeploymentStatus;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                write!(formatter, "expected one of: {:?}", &FIELDS)
            }

            fn visit_i64<E>(self, v: i64) -> std::result::Result<Self::Value, E>
            where
                E: serde::de::Error,
            {
                i32::try_from(v)
                    .ok()
                    .and_then(|x| x.try_into().ok())
                    .ok_or_else(|| {
                        serde::de::Error::invalid_value(serde::de::Unexpected::Signed(v), &self)
                    })
            }

            fn visit_u64<E>(self, v: u64) -> std::result::Result<Self::Value, E>
            where
                E: serde::de::Error,
            {
                i32::try_from(v)
                    .ok()
                    .and_then(|x| x.try_into().ok())
                    .ok_or_else(|| {
                        serde::de::Error::invalid_value(serde::de::Unexpected::Unsigned(v), &self)
                    })
            }

            fn visit_str<E>(self, value: &str) -> std::result::Result<Self::Value, E>
            where
                E: serde::de::Error,
            {
                match value {
                    "DEPLOYMENT_STATUS_UNSPECIFIED" => Ok(DeploymentStatus::Unspecified),
                    "DEPLOYMENT_STATUS_PLANNED" => Ok(DeploymentStatus::Planned),
                    "DEPLOYMENT_STATUS_GRAY" => Ok(DeploymentStatus::Gray),
                    "DEPLOYMENT_STATUS_ACTIVE" => Ok(DeploymentStatus::Active),
                    "DEPLOYMENT_STATUS_PAUSED" => Ok(DeploymentStatus::Paused),
                    "DEPLOYMENT_STATUS_ROLLED_BACK" => Ok(DeploymentStatus::RolledBack),
                    _ => Err(serde::de::Error::unknown_variant(value, FIELDS)),
                }
            }
        }
        deserializer.deserialize_any(GeneratedVisitor)
    }
}
impl serde::Serialize for DetectionBatch {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.behaviors.is_empty() {
            len += 1;
        }
        if !self.businesses.is_empty() {
            len += 1;
        }
        if !self.batch_id.is_empty() {
            len += 1;
        }
        if !self.tenant_id.is_empty() {
            len += 1;
        }
        if !self.run_id.is_empty() {
            len += 1;
        }
        if self.created_at != 0 {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.DetectionBatch", len)?;
        if !self.behaviors.is_empty() {
            struct_ser.serialize_field("behaviors", &self.behaviors)?;
        }
        if !self.businesses.is_empty() {
            struct_ser.serialize_field("businesses", &self.businesses)?;
        }
        if !self.batch_id.is_empty() {
            struct_ser.serialize_field("batchId", &self.batch_id)?;
        }
        if !self.tenant_id.is_empty() {
            struct_ser.serialize_field("tenantId", &self.tenant_id)?;
        }
        if !self.run_id.is_empty() {
            struct_ser.serialize_field("runId", &self.run_id)?;
        }
        if self.created_at != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("createdAt", ToString::to_string(&self.created_at).as_str())?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for DetectionBatch {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "behaviors",
            "businesses",
            "batch_id",
            "batchId",
            "tenant_id",
            "tenantId",
            "run_id",
            "runId",
            "created_at",
            "createdAt",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Behaviors,
            Businesses,
            BatchId,
            TenantId,
            RunId,
            CreatedAt,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "behaviors" => Ok(GeneratedField::Behaviors),
                            "businesses" => Ok(GeneratedField::Businesses),
                            "batchId" | "batch_id" => Ok(GeneratedField::BatchId),
                            "tenantId" | "tenant_id" => Ok(GeneratedField::TenantId),
                            "runId" | "run_id" => Ok(GeneratedField::RunId),
                            "createdAt" | "created_at" => Ok(GeneratedField::CreatedAt),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = DetectionBatch;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.DetectionBatch")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<DetectionBatch, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut behaviors__ = None;
                let mut businesses__ = None;
                let mut batch_id__ = None;
                let mut tenant_id__ = None;
                let mut run_id__ = None;
                let mut created_at__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Behaviors => {
                            if behaviors__.is_some() {
                                return Err(serde::de::Error::duplicate_field("behaviors"));
                            }
                            behaviors__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Businesses => {
                            if businesses__.is_some() {
                                return Err(serde::de::Error::duplicate_field("businesses"));
                            }
                            businesses__ = Some(map_.next_value()?);
                        }
                        GeneratedField::BatchId => {
                            if batch_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("batchId"));
                            }
                            batch_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::TenantId => {
                            if tenant_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("tenantId"));
                            }
                            tenant_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::RunId => {
                            if run_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("runId"));
                            }
                            run_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::CreatedAt => {
                            if created_at__.is_some() {
                                return Err(serde::de::Error::duplicate_field("createdAt"));
                            }
                            created_at__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                    }
                }
                Ok(DetectionBatch {
                    behaviors: behaviors__.unwrap_or_default(),
                    businesses: businesses__.unwrap_or_default(),
                    batch_id: batch_id__.unwrap_or_default(),
                    tenant_id: tenant_id__.unwrap_or_default(),
                    run_id: run_id__.unwrap_or_default(),
                    created_at: created_at__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.DetectionBatch", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for DetectionBehavior {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if self.header.is_some() {
            len += 1;
        }
        if !self.model_version.is_empty() {
            len += 1;
        }
        if !self.community_id.is_empty() {
            len += 1;
        }
        if !self.object_type.is_empty() {
            len += 1;
        }
        if !self.object_id.is_empty() {
            len += 1;
        }
        if self.ts != 0 {
            len += 1;
        }
        if !self.labels.is_empty() {
            len += 1;
        }
        if !self.scores.is_empty() {
            len += 1;
        }
        if !self.top_label.is_empty() {
            len += 1;
        }
        if self.top_score != 0. {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.DetectionBehavior", len)?;
        if let Some(v) = self.header.as_ref() {
            struct_ser.serialize_field("header", v)?;
        }
        if !self.model_version.is_empty() {
            struct_ser.serialize_field("modelVersion", &self.model_version)?;
        }
        if !self.community_id.is_empty() {
            struct_ser.serialize_field("communityId", &self.community_id)?;
        }
        if !self.object_type.is_empty() {
            struct_ser.serialize_field("objectType", &self.object_type)?;
        }
        if !self.object_id.is_empty() {
            struct_ser.serialize_field("objectId", &self.object_id)?;
        }
        if self.ts != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("ts", ToString::to_string(&self.ts).as_str())?;
        }
        if !self.labels.is_empty() {
            struct_ser.serialize_field("labels", &self.labels)?;
        }
        if !self.scores.is_empty() {
            struct_ser.serialize_field("scores", &self.scores)?;
        }
        if !self.top_label.is_empty() {
            struct_ser.serialize_field("topLabel", &self.top_label)?;
        }
        if self.top_score != 0. {
            struct_ser.serialize_field("topScore", &self.top_score)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for DetectionBehavior {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "header",
            "model_version",
            "modelVersion",
            "community_id",
            "communityId",
            "object_type",
            "objectType",
            "object_id",
            "objectId",
            "ts",
            "labels",
            "scores",
            "top_label",
            "topLabel",
            "top_score",
            "topScore",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Header,
            ModelVersion,
            CommunityId,
            ObjectType,
            ObjectId,
            Ts,
            Labels,
            Scores,
            TopLabel,
            TopScore,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "header" => Ok(GeneratedField::Header),
                            "modelVersion" | "model_version" => Ok(GeneratedField::ModelVersion),
                            "communityId" | "community_id" => Ok(GeneratedField::CommunityId),
                            "objectType" | "object_type" => Ok(GeneratedField::ObjectType),
                            "objectId" | "object_id" => Ok(GeneratedField::ObjectId),
                            "ts" => Ok(GeneratedField::Ts),
                            "labels" => Ok(GeneratedField::Labels),
                            "scores" => Ok(GeneratedField::Scores),
                            "topLabel" | "top_label" => Ok(GeneratedField::TopLabel),
                            "topScore" | "top_score" => Ok(GeneratedField::TopScore),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = DetectionBehavior;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.DetectionBehavior")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<DetectionBehavior, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut header__ = None;
                let mut model_version__ = None;
                let mut community_id__ = None;
                let mut object_type__ = None;
                let mut object_id__ = None;
                let mut ts__ = None;
                let mut labels__ = None;
                let mut scores__ = None;
                let mut top_label__ = None;
                let mut top_score__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Header => {
                            if header__.is_some() {
                                return Err(serde::de::Error::duplicate_field("header"));
                            }
                            header__ = map_.next_value()?;
                        }
                        GeneratedField::ModelVersion => {
                            if model_version__.is_some() {
                                return Err(serde::de::Error::duplicate_field("modelVersion"));
                            }
                            model_version__ = Some(map_.next_value()?);
                        }
                        GeneratedField::CommunityId => {
                            if community_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("communityId"));
                            }
                            community_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::ObjectType => {
                            if object_type__.is_some() {
                                return Err(serde::de::Error::duplicate_field("objectType"));
                            }
                            object_type__ = Some(map_.next_value()?);
                        }
                        GeneratedField::ObjectId => {
                            if object_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("objectId"));
                            }
                            object_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Ts => {
                            if ts__.is_some() {
                                return Err(serde::de::Error::duplicate_field("ts"));
                            }
                            ts__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::Labels => {
                            if labels__.is_some() {
                                return Err(serde::de::Error::duplicate_field("labels"));
                            }
                            labels__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Scores => {
                            if scores__.is_some() {
                                return Err(serde::de::Error::duplicate_field("scores"));
                            }
                            scores__ = 
                                Some(map_.next_value::<Vec<::pbjson::private::NumberDeserialize<_>>>()?
                                    .into_iter().map(|x| x.0).collect())
                            ;
                        }
                        GeneratedField::TopLabel => {
                            if top_label__.is_some() {
                                return Err(serde::de::Error::duplicate_field("topLabel"));
                            }
                            top_label__ = Some(map_.next_value()?);
                        }
                        GeneratedField::TopScore => {
                            if top_score__.is_some() {
                                return Err(serde::de::Error::duplicate_field("topScore"));
                            }
                            top_score__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                    }
                }
                Ok(DetectionBehavior {
                    header: header__,
                    model_version: model_version__.unwrap_or_default(),
                    community_id: community_id__.unwrap_or_default(),
                    object_type: object_type__.unwrap_or_default(),
                    object_id: object_id__.unwrap_or_default(),
                    ts: ts__.unwrap_or_default(),
                    labels: labels__.unwrap_or_default(),
                    scores: scores__.unwrap_or_default(),
                    top_label: top_label__.unwrap_or_default(),
                    top_score: top_score__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.DetectionBehavior", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for DetectionBusiness {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if self.header.is_some() {
            len += 1;
        }
        if !self.model_version.is_empty() {
            len += 1;
        }
        if !self.rule_version.is_empty() {
            len += 1;
        }
        if self.ts != 0 {
            len += 1;
        }
        if !self.community_id.is_empty() {
            len += 1;
        }
        if !self.session_id.is_empty() {
            len += 1;
        }
        if !self.campaign_id.is_empty() {
            len += 1;
        }
        if !self.detection_type.is_empty() {
            len += 1;
        }
        if !self.label.is_empty() {
            len += 1;
        }
        if self.score != 0. {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.DetectionBusiness", len)?;
        if let Some(v) = self.header.as_ref() {
            struct_ser.serialize_field("header", v)?;
        }
        if !self.model_version.is_empty() {
            struct_ser.serialize_field("modelVersion", &self.model_version)?;
        }
        if !self.rule_version.is_empty() {
            struct_ser.serialize_field("ruleVersion", &self.rule_version)?;
        }
        if self.ts != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("ts", ToString::to_string(&self.ts).as_str())?;
        }
        if !self.community_id.is_empty() {
            struct_ser.serialize_field("communityId", &self.community_id)?;
        }
        if !self.session_id.is_empty() {
            struct_ser.serialize_field("sessionId", &self.session_id)?;
        }
        if !self.campaign_id.is_empty() {
            struct_ser.serialize_field("campaignId", &self.campaign_id)?;
        }
        if !self.detection_type.is_empty() {
            struct_ser.serialize_field("detectionType", &self.detection_type)?;
        }
        if !self.label.is_empty() {
            struct_ser.serialize_field("label", &self.label)?;
        }
        if self.score != 0. {
            struct_ser.serialize_field("score", &self.score)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for DetectionBusiness {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "header",
            "model_version",
            "modelVersion",
            "rule_version",
            "ruleVersion",
            "ts",
            "community_id",
            "communityId",
            "session_id",
            "sessionId",
            "campaign_id",
            "campaignId",
            "detection_type",
            "detectionType",
            "label",
            "score",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Header,
            ModelVersion,
            RuleVersion,
            Ts,
            CommunityId,
            SessionId,
            CampaignId,
            DetectionType,
            Label,
            Score,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "header" => Ok(GeneratedField::Header),
                            "modelVersion" | "model_version" => Ok(GeneratedField::ModelVersion),
                            "ruleVersion" | "rule_version" => Ok(GeneratedField::RuleVersion),
                            "ts" => Ok(GeneratedField::Ts),
                            "communityId" | "community_id" => Ok(GeneratedField::CommunityId),
                            "sessionId" | "session_id" => Ok(GeneratedField::SessionId),
                            "campaignId" | "campaign_id" => Ok(GeneratedField::CampaignId),
                            "detectionType" | "detection_type" => Ok(GeneratedField::DetectionType),
                            "label" => Ok(GeneratedField::Label),
                            "score" => Ok(GeneratedField::Score),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = DetectionBusiness;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.DetectionBusiness")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<DetectionBusiness, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut header__ = None;
                let mut model_version__ = None;
                let mut rule_version__ = None;
                let mut ts__ = None;
                let mut community_id__ = None;
                let mut session_id__ = None;
                let mut campaign_id__ = None;
                let mut detection_type__ = None;
                let mut label__ = None;
                let mut score__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Header => {
                            if header__.is_some() {
                                return Err(serde::de::Error::duplicate_field("header"));
                            }
                            header__ = map_.next_value()?;
                        }
                        GeneratedField::ModelVersion => {
                            if model_version__.is_some() {
                                return Err(serde::de::Error::duplicate_field("modelVersion"));
                            }
                            model_version__ = Some(map_.next_value()?);
                        }
                        GeneratedField::RuleVersion => {
                            if rule_version__.is_some() {
                                return Err(serde::de::Error::duplicate_field("ruleVersion"));
                            }
                            rule_version__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Ts => {
                            if ts__.is_some() {
                                return Err(serde::de::Error::duplicate_field("ts"));
                            }
                            ts__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::CommunityId => {
                            if community_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("communityId"));
                            }
                            community_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::SessionId => {
                            if session_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("sessionId"));
                            }
                            session_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::CampaignId => {
                            if campaign_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("campaignId"));
                            }
                            campaign_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::DetectionType => {
                            if detection_type__.is_some() {
                                return Err(serde::de::Error::duplicate_field("detectionType"));
                            }
                            detection_type__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Label => {
                            if label__.is_some() {
                                return Err(serde::de::Error::duplicate_field("label"));
                            }
                            label__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Score => {
                            if score__.is_some() {
                                return Err(serde::de::Error::duplicate_field("score"));
                            }
                            score__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                    }
                }
                Ok(DetectionBusiness {
                    header: header__,
                    model_version: model_version__.unwrap_or_default(),
                    rule_version: rule_version__.unwrap_or_default(),
                    ts: ts__.unwrap_or_default(),
                    community_id: community_id__.unwrap_or_default(),
                    session_id: session_id__.unwrap_or_default(),
                    campaign_id: campaign_id__.unwrap_or_default(),
                    detection_type: detection_type__.unwrap_or_default(),
                    label: label__.unwrap_or_default(),
                    score: score__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.DetectionBusiness", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for DeviceLog {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.log_id.is_empty() {
            len += 1;
        }
        if !self.tenant_id.is_empty() {
            len += 1;
        }
        if !self.device_ip.is_empty() {
            len += 1;
        }
        if !self.device_type.is_empty() {
            len += 1;
        }
        if self.facility != 0 {
            len += 1;
        }
        if self.severity != 0 {
            len += 1;
        }
        if self.timestamp != 0 {
            len += 1;
        }
        if !self.message.is_empty() {
            len += 1;
        }
        if !self.parsed.is_empty() {
            len += 1;
        }
        if !self.source.is_empty() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.DeviceLog", len)?;
        if !self.log_id.is_empty() {
            struct_ser.serialize_field("logId", &self.log_id)?;
        }
        if !self.tenant_id.is_empty() {
            struct_ser.serialize_field("tenantId", &self.tenant_id)?;
        }
        if !self.device_ip.is_empty() {
            struct_ser.serialize_field("deviceIp", &self.device_ip)?;
        }
        if !self.device_type.is_empty() {
            struct_ser.serialize_field("deviceType", &self.device_type)?;
        }
        if self.facility != 0 {
            struct_ser.serialize_field("facility", &self.facility)?;
        }
        if self.severity != 0 {
            struct_ser.serialize_field("severity", &self.severity)?;
        }
        if self.timestamp != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("timestamp", ToString::to_string(&self.timestamp).as_str())?;
        }
        if !self.message.is_empty() {
            struct_ser.serialize_field("message", &self.message)?;
        }
        if !self.parsed.is_empty() {
            struct_ser.serialize_field("parsed", &self.parsed)?;
        }
        if !self.source.is_empty() {
            struct_ser.serialize_field("source", &self.source)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for DeviceLog {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "log_id",
            "logId",
            "tenant_id",
            "tenantId",
            "device_ip",
            "deviceIp",
            "device_type",
            "deviceType",
            "facility",
            "severity",
            "timestamp",
            "message",
            "parsed",
            "source",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            LogId,
            TenantId,
            DeviceIp,
            DeviceType,
            Facility,
            Severity,
            Timestamp,
            Message,
            Parsed,
            Source,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "logId" | "log_id" => Ok(GeneratedField::LogId),
                            "tenantId" | "tenant_id" => Ok(GeneratedField::TenantId),
                            "deviceIp" | "device_ip" => Ok(GeneratedField::DeviceIp),
                            "deviceType" | "device_type" => Ok(GeneratedField::DeviceType),
                            "facility" => Ok(GeneratedField::Facility),
                            "severity" => Ok(GeneratedField::Severity),
                            "timestamp" => Ok(GeneratedField::Timestamp),
                            "message" => Ok(GeneratedField::Message),
                            "parsed" => Ok(GeneratedField::Parsed),
                            "source" => Ok(GeneratedField::Source),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = DeviceLog;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.DeviceLog")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<DeviceLog, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut log_id__ = None;
                let mut tenant_id__ = None;
                let mut device_ip__ = None;
                let mut device_type__ = None;
                let mut facility__ = None;
                let mut severity__ = None;
                let mut timestamp__ = None;
                let mut message__ = None;
                let mut parsed__ = None;
                let mut source__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::LogId => {
                            if log_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("logId"));
                            }
                            log_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::TenantId => {
                            if tenant_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("tenantId"));
                            }
                            tenant_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::DeviceIp => {
                            if device_ip__.is_some() {
                                return Err(serde::de::Error::duplicate_field("deviceIp"));
                            }
                            device_ip__ = Some(map_.next_value()?);
                        }
                        GeneratedField::DeviceType => {
                            if device_type__.is_some() {
                                return Err(serde::de::Error::duplicate_field("deviceType"));
                            }
                            device_type__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Facility => {
                            if facility__.is_some() {
                                return Err(serde::de::Error::duplicate_field("facility"));
                            }
                            facility__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::Severity => {
                            if severity__.is_some() {
                                return Err(serde::de::Error::duplicate_field("severity"));
                            }
                            severity__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::Timestamp => {
                            if timestamp__.is_some() {
                                return Err(serde::de::Error::duplicate_field("timestamp"));
                            }
                            timestamp__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::Message => {
                            if message__.is_some() {
                                return Err(serde::de::Error::duplicate_field("message"));
                            }
                            message__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Parsed => {
                            if parsed__.is_some() {
                                return Err(serde::de::Error::duplicate_field("parsed"));
                            }
                            parsed__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Source => {
                            if source__.is_some() {
                                return Err(serde::de::Error::duplicate_field("source"));
                            }
                            source__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(DeviceLog {
                    log_id: log_id__.unwrap_or_default(),
                    tenant_id: tenant_id__.unwrap_or_default(),
                    device_ip: device_ip__.unwrap_or_default(),
                    device_type: device_type__.unwrap_or_default(),
                    facility: facility__.unwrap_or_default(),
                    severity: severity__.unwrap_or_default(),
                    timestamp: timestamp__.unwrap_or_default(),
                    message: message__.unwrap_or_default(),
                    parsed: parsed__.unwrap_or_default(),
                    source: source__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.DeviceLog", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for DeviceLogBatch {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.events.is_empty() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.DeviceLogBatch", len)?;
        if !self.events.is_empty() {
            struct_ser.serialize_field("events", &self.events)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for DeviceLogBatch {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "events",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Events,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "events" => Ok(GeneratedField::Events),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = DeviceLogBatch;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.DeviceLogBatch")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<DeviceLogBatch, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut events__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Events => {
                            if events__.is_some() {
                                return Err(serde::de::Error::duplicate_field("events"));
                            }
                            events__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(DeviceLogBatch {
                    events: events__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.DeviceLogBatch", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for EventHeader {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.event_id.is_empty() {
            len += 1;
        }
        if !self.tenant_id.is_empty() {
            len += 1;
        }
        if !self.run_id.is_empty() {
            len += 1;
        }
        if self.event_ts != 0 {
            len += 1;
        }
        if self.ingest_ts != 0 {
            len += 1;
        }
        if !self.probe_id.is_empty() {
            len += 1;
        }
        if !self.feature_set_id.is_empty() {
            len += 1;
        }
        if self.kafka_ts != 0 {
            len += 1;
        }
        if self.flink_out_ts != 0 {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.EventHeader", len)?;
        if !self.event_id.is_empty() {
            struct_ser.serialize_field("eventId", &self.event_id)?;
        }
        if !self.tenant_id.is_empty() {
            struct_ser.serialize_field("tenantId", &self.tenant_id)?;
        }
        if !self.run_id.is_empty() {
            struct_ser.serialize_field("runId", &self.run_id)?;
        }
        if self.event_ts != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("eventTs", ToString::to_string(&self.event_ts).as_str())?;
        }
        if self.ingest_ts != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("ingestTs", ToString::to_string(&self.ingest_ts).as_str())?;
        }
        if !self.probe_id.is_empty() {
            struct_ser.serialize_field("probeId", &self.probe_id)?;
        }
        if !self.feature_set_id.is_empty() {
            struct_ser.serialize_field("featureSetId", &self.feature_set_id)?;
        }
        if self.kafka_ts != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("kafkaTs", ToString::to_string(&self.kafka_ts).as_str())?;
        }
        if self.flink_out_ts != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("flinkOutTs", ToString::to_string(&self.flink_out_ts).as_str())?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for EventHeader {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "event_id",
            "eventId",
            "tenant_id",
            "tenantId",
            "run_id",
            "runId",
            "event_ts",
            "eventTs",
            "ingest_ts",
            "ingestTs",
            "probe_id",
            "probeId",
            "feature_set_id",
            "featureSetId",
            "kafka_ts",
            "kafkaTs",
            "flink_out_ts",
            "flinkOutTs",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            EventId,
            TenantId,
            RunId,
            EventTs,
            IngestTs,
            ProbeId,
            FeatureSetId,
            KafkaTs,
            FlinkOutTs,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "eventId" | "event_id" => Ok(GeneratedField::EventId),
                            "tenantId" | "tenant_id" => Ok(GeneratedField::TenantId),
                            "runId" | "run_id" => Ok(GeneratedField::RunId),
                            "eventTs" | "event_ts" => Ok(GeneratedField::EventTs),
                            "ingestTs" | "ingest_ts" => Ok(GeneratedField::IngestTs),
                            "probeId" | "probe_id" => Ok(GeneratedField::ProbeId),
                            "featureSetId" | "feature_set_id" => Ok(GeneratedField::FeatureSetId),
                            "kafkaTs" | "kafka_ts" => Ok(GeneratedField::KafkaTs),
                            "flinkOutTs" | "flink_out_ts" => Ok(GeneratedField::FlinkOutTs),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = EventHeader;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.EventHeader")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<EventHeader, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut event_id__ = None;
                let mut tenant_id__ = None;
                let mut run_id__ = None;
                let mut event_ts__ = None;
                let mut ingest_ts__ = None;
                let mut probe_id__ = None;
                let mut feature_set_id__ = None;
                let mut kafka_ts__ = None;
                let mut flink_out_ts__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::EventId => {
                            if event_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("eventId"));
                            }
                            event_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::TenantId => {
                            if tenant_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("tenantId"));
                            }
                            tenant_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::RunId => {
                            if run_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("runId"));
                            }
                            run_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::EventTs => {
                            if event_ts__.is_some() {
                                return Err(serde::de::Error::duplicate_field("eventTs"));
                            }
                            event_ts__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::IngestTs => {
                            if ingest_ts__.is_some() {
                                return Err(serde::de::Error::duplicate_field("ingestTs"));
                            }
                            ingest_ts__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::ProbeId => {
                            if probe_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("probeId"));
                            }
                            probe_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::FeatureSetId => {
                            if feature_set_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("featureSetId"));
                            }
                            feature_set_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::KafkaTs => {
                            if kafka_ts__.is_some() {
                                return Err(serde::de::Error::duplicate_field("kafkaTs"));
                            }
                            kafka_ts__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::FlinkOutTs => {
                            if flink_out_ts__.is_some() {
                                return Err(serde::de::Error::duplicate_field("flinkOutTs"));
                            }
                            flink_out_ts__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                    }
                }
                Ok(EventHeader {
                    event_id: event_id__.unwrap_or_default(),
                    tenant_id: tenant_id__.unwrap_or_default(),
                    run_id: run_id__.unwrap_or_default(),
                    event_ts: event_ts__.unwrap_or_default(),
                    ingest_ts: ingest_ts__.unwrap_or_default(),
                    probe_id: probe_id__.unwrap_or_default(),
                    feature_set_id: feature_set_id__.unwrap_or_default(),
                    kafka_ts: kafka_ts__.unwrap_or_default(),
                    flink_out_ts: flink_out_ts__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.EventHeader", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for Evidence {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.tenant_id.is_empty() {
            len += 1;
        }
        if !self.evidence_id.is_empty() {
            len += 1;
        }
        if !self.alert_id.is_empty() {
            len += 1;
        }
        if self.ts != 0 {
            len += 1;
        }
        if !self.r#type.is_empty() {
            len += 1;
        }
        if !self.summary.is_empty() {
            len += 1;
        }
        if !self.metrics_json.is_empty() {
            len += 1;
        }
        if !self.snippet_ref_json.is_empty() {
            len += 1;
        }
        if !self.arkime_link.is_empty() {
            len += 1;
        }
        if self.confidence != 0. {
            len += 1;
        }
        if !self.event_id.is_empty() {
            len += 1;
        }
        if self.ingest_ts != 0 {
            len += 1;
        }
        if !self.visualization_url.is_empty() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.Evidence", len)?;
        if !self.tenant_id.is_empty() {
            struct_ser.serialize_field("tenantId", &self.tenant_id)?;
        }
        if !self.evidence_id.is_empty() {
            struct_ser.serialize_field("evidenceId", &self.evidence_id)?;
        }
        if !self.alert_id.is_empty() {
            struct_ser.serialize_field("alertId", &self.alert_id)?;
        }
        if self.ts != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("ts", ToString::to_string(&self.ts).as_str())?;
        }
        if !self.r#type.is_empty() {
            struct_ser.serialize_field("type", &self.r#type)?;
        }
        if !self.summary.is_empty() {
            struct_ser.serialize_field("summary", &self.summary)?;
        }
        if !self.metrics_json.is_empty() {
            struct_ser.serialize_field("metricsJson", &self.metrics_json)?;
        }
        if !self.snippet_ref_json.is_empty() {
            struct_ser.serialize_field("snippetRefJson", &self.snippet_ref_json)?;
        }
        if !self.arkime_link.is_empty() {
            struct_ser.serialize_field("arkimeLink", &self.arkime_link)?;
        }
        if self.confidence != 0. {
            struct_ser.serialize_field("confidence", &self.confidence)?;
        }
        if !self.event_id.is_empty() {
            struct_ser.serialize_field("eventId", &self.event_id)?;
        }
        if self.ingest_ts != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("ingestTs", ToString::to_string(&self.ingest_ts).as_str())?;
        }
        if !self.visualization_url.is_empty() {
            struct_ser.serialize_field("visualizationUrl", &self.visualization_url)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for Evidence {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "tenant_id",
            "tenantId",
            "evidence_id",
            "evidenceId",
            "alert_id",
            "alertId",
            "ts",
            "type",
            "summary",
            "metrics_json",
            "metricsJson",
            "snippet_ref_json",
            "snippetRefJson",
            "arkime_link",
            "arkimeLink",
            "confidence",
            "event_id",
            "eventId",
            "ingest_ts",
            "ingestTs",
            "visualization_url",
            "visualizationUrl",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            TenantId,
            EvidenceId,
            AlertId,
            Ts,
            Type,
            Summary,
            MetricsJson,
            SnippetRefJson,
            ArkimeLink,
            Confidence,
            EventId,
            IngestTs,
            VisualizationUrl,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "tenantId" | "tenant_id" => Ok(GeneratedField::TenantId),
                            "evidenceId" | "evidence_id" => Ok(GeneratedField::EvidenceId),
                            "alertId" | "alert_id" => Ok(GeneratedField::AlertId),
                            "ts" => Ok(GeneratedField::Ts),
                            "type" => Ok(GeneratedField::Type),
                            "summary" => Ok(GeneratedField::Summary),
                            "metricsJson" | "metrics_json" => Ok(GeneratedField::MetricsJson),
                            "snippetRefJson" | "snippet_ref_json" => Ok(GeneratedField::SnippetRefJson),
                            "arkimeLink" | "arkime_link" => Ok(GeneratedField::ArkimeLink),
                            "confidence" => Ok(GeneratedField::Confidence),
                            "eventId" | "event_id" => Ok(GeneratedField::EventId),
                            "ingestTs" | "ingest_ts" => Ok(GeneratedField::IngestTs),
                            "visualizationUrl" | "visualization_url" => Ok(GeneratedField::VisualizationUrl),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = Evidence;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.Evidence")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<Evidence, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut tenant_id__ = None;
                let mut evidence_id__ = None;
                let mut alert_id__ = None;
                let mut ts__ = None;
                let mut r#type__ = None;
                let mut summary__ = None;
                let mut metrics_json__ = None;
                let mut snippet_ref_json__ = None;
                let mut arkime_link__ = None;
                let mut confidence__ = None;
                let mut event_id__ = None;
                let mut ingest_ts__ = None;
                let mut visualization_url__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::TenantId => {
                            if tenant_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("tenantId"));
                            }
                            tenant_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::EvidenceId => {
                            if evidence_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("evidenceId"));
                            }
                            evidence_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::AlertId => {
                            if alert_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("alertId"));
                            }
                            alert_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Ts => {
                            if ts__.is_some() {
                                return Err(serde::de::Error::duplicate_field("ts"));
                            }
                            ts__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::Type => {
                            if r#type__.is_some() {
                                return Err(serde::de::Error::duplicate_field("type"));
                            }
                            r#type__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Summary => {
                            if summary__.is_some() {
                                return Err(serde::de::Error::duplicate_field("summary"));
                            }
                            summary__ = Some(map_.next_value()?);
                        }
                        GeneratedField::MetricsJson => {
                            if metrics_json__.is_some() {
                                return Err(serde::de::Error::duplicate_field("metricsJson"));
                            }
                            metrics_json__ = Some(map_.next_value()?);
                        }
                        GeneratedField::SnippetRefJson => {
                            if snippet_ref_json__.is_some() {
                                return Err(serde::de::Error::duplicate_field("snippetRefJson"));
                            }
                            snippet_ref_json__ = Some(map_.next_value()?);
                        }
                        GeneratedField::ArkimeLink => {
                            if arkime_link__.is_some() {
                                return Err(serde::de::Error::duplicate_field("arkimeLink"));
                            }
                            arkime_link__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Confidence => {
                            if confidence__.is_some() {
                                return Err(serde::de::Error::duplicate_field("confidence"));
                            }
                            confidence__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::EventId => {
                            if event_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("eventId"));
                            }
                            event_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::IngestTs => {
                            if ingest_ts__.is_some() {
                                return Err(serde::de::Error::duplicate_field("ingestTs"));
                            }
                            ingest_ts__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::VisualizationUrl => {
                            if visualization_url__.is_some() {
                                return Err(serde::de::Error::duplicate_field("visualizationUrl"));
                            }
                            visualization_url__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(Evidence {
                    tenant_id: tenant_id__.unwrap_or_default(),
                    evidence_id: evidence_id__.unwrap_or_default(),
                    alert_id: alert_id__.unwrap_or_default(),
                    ts: ts__.unwrap_or_default(),
                    r#type: r#type__.unwrap_or_default(),
                    summary: summary__.unwrap_or_default(),
                    metrics_json: metrics_json__.unwrap_or_default(),
                    snippet_ref_json: snippet_ref_json__.unwrap_or_default(),
                    arkime_link: arkime_link__.unwrap_or_default(),
                    confidence: confidence__.unwrap_or_default(),
                    event_id: event_id__.unwrap_or_default(),
                    ingest_ts: ingest_ts__.unwrap_or_default(),
                    visualization_url: visualization_url__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.Evidence", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for FeatureBatch {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.stats.is_empty() {
            len += 1;
        }
        if !self.sequences.is_empty() {
            len += 1;
        }
        if !self.fingerprints.is_empty() {
            len += 1;
        }
        if !self.batch_id.is_empty() {
            len += 1;
        }
        if !self.tenant_id.is_empty() {
            len += 1;
        }
        if !self.run_id.is_empty() {
            len += 1;
        }
        if self.created_at != 0 {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.FeatureBatch", len)?;
        if !self.stats.is_empty() {
            struct_ser.serialize_field("stats", &self.stats)?;
        }
        if !self.sequences.is_empty() {
            struct_ser.serialize_field("sequences", &self.sequences)?;
        }
        if !self.fingerprints.is_empty() {
            struct_ser.serialize_field("fingerprints", &self.fingerprints)?;
        }
        if !self.batch_id.is_empty() {
            struct_ser.serialize_field("batchId", &self.batch_id)?;
        }
        if !self.tenant_id.is_empty() {
            struct_ser.serialize_field("tenantId", &self.tenant_id)?;
        }
        if !self.run_id.is_empty() {
            struct_ser.serialize_field("runId", &self.run_id)?;
        }
        if self.created_at != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("createdAt", ToString::to_string(&self.created_at).as_str())?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for FeatureBatch {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "stats",
            "sequences",
            "fingerprints",
            "batch_id",
            "batchId",
            "tenant_id",
            "tenantId",
            "run_id",
            "runId",
            "created_at",
            "createdAt",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Stats,
            Sequences,
            Fingerprints,
            BatchId,
            TenantId,
            RunId,
            CreatedAt,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "stats" => Ok(GeneratedField::Stats),
                            "sequences" => Ok(GeneratedField::Sequences),
                            "fingerprints" => Ok(GeneratedField::Fingerprints),
                            "batchId" | "batch_id" => Ok(GeneratedField::BatchId),
                            "tenantId" | "tenant_id" => Ok(GeneratedField::TenantId),
                            "runId" | "run_id" => Ok(GeneratedField::RunId),
                            "createdAt" | "created_at" => Ok(GeneratedField::CreatedAt),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = FeatureBatch;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.FeatureBatch")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<FeatureBatch, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut stats__ = None;
                let mut sequences__ = None;
                let mut fingerprints__ = None;
                let mut batch_id__ = None;
                let mut tenant_id__ = None;
                let mut run_id__ = None;
                let mut created_at__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Stats => {
                            if stats__.is_some() {
                                return Err(serde::de::Error::duplicate_field("stats"));
                            }
                            stats__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Sequences => {
                            if sequences__.is_some() {
                                return Err(serde::de::Error::duplicate_field("sequences"));
                            }
                            sequences__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Fingerprints => {
                            if fingerprints__.is_some() {
                                return Err(serde::de::Error::duplicate_field("fingerprints"));
                            }
                            fingerprints__ = Some(map_.next_value()?);
                        }
                        GeneratedField::BatchId => {
                            if batch_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("batchId"));
                            }
                            batch_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::TenantId => {
                            if tenant_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("tenantId"));
                            }
                            tenant_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::RunId => {
                            if run_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("runId"));
                            }
                            run_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::CreatedAt => {
                            if created_at__.is_some() {
                                return Err(serde::de::Error::duplicate_field("createdAt"));
                            }
                            created_at__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                    }
                }
                Ok(FeatureBatch {
                    stats: stats__.unwrap_or_default(),
                    sequences: sequences__.unwrap_or_default(),
                    fingerprints: fingerprints__.unwrap_or_default(),
                    batch_id: batch_id__.unwrap_or_default(),
                    tenant_id: tenant_id__.unwrap_or_default(),
                    run_id: run_id__.unwrap_or_default(),
                    created_at: created_at__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.FeatureBatch", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for FeatureFingerprint {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if self.header.is_some() {
            len += 1;
        }
        if !self.community_id.is_empty() {
            len += 1;
        }
        if !self.session_id.is_empty() {
            len += 1;
        }
        if self.ts != 0 {
            len += 1;
        }
        if self.is_encrypted != 0 {
            len += 1;
        }
        if !self.tls_version.is_empty() {
            len += 1;
        }
        if !self.ja3.is_empty() {
            len += 1;
        }
        if !self.sni_hash.is_empty() {
            len += 1;
        }
        if !self.cert_sha256.is_empty() {
            len += 1;
        }
        if self.cert_is_self_signed != 0 {
            len += 1;
        }
        if self.pubkey_len != 0 {
            len += 1;
        }
        if !self.hex_freq.is_empty() {
            len += 1;
        }
        if !self.hex_ratio.is_empty() {
            len += 1;
        }
        if self.entropy_payload != 0. {
            len += 1;
        }
        if self.chi_square_bfd != 0. {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.FeatureFingerprint", len)?;
        if let Some(v) = self.header.as_ref() {
            struct_ser.serialize_field("header", v)?;
        }
        if !self.community_id.is_empty() {
            struct_ser.serialize_field("communityId", &self.community_id)?;
        }
        if !self.session_id.is_empty() {
            struct_ser.serialize_field("sessionId", &self.session_id)?;
        }
        if self.ts != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("ts", ToString::to_string(&self.ts).as_str())?;
        }
        if self.is_encrypted != 0 {
            struct_ser.serialize_field("isEncrypted", &self.is_encrypted)?;
        }
        if !self.tls_version.is_empty() {
            struct_ser.serialize_field("tlsVersion", &self.tls_version)?;
        }
        if !self.ja3.is_empty() {
            struct_ser.serialize_field("ja3", &self.ja3)?;
        }
        if !self.sni_hash.is_empty() {
            struct_ser.serialize_field("sniHash", &self.sni_hash)?;
        }
        if !self.cert_sha256.is_empty() {
            struct_ser.serialize_field("certSha256", &self.cert_sha256)?;
        }
        if self.cert_is_self_signed != 0 {
            struct_ser.serialize_field("certIsSelfSigned", &self.cert_is_self_signed)?;
        }
        if self.pubkey_len != 0 {
            struct_ser.serialize_field("pubkeyLen", &self.pubkey_len)?;
        }
        if !self.hex_freq.is_empty() {
            struct_ser.serialize_field("hexFreq", &self.hex_freq)?;
        }
        if !self.hex_ratio.is_empty() {
            struct_ser.serialize_field("hexRatio", &self.hex_ratio)?;
        }
        if self.entropy_payload != 0. {
            struct_ser.serialize_field("entropyPayload", &self.entropy_payload)?;
        }
        if self.chi_square_bfd != 0. {
            struct_ser.serialize_field("chiSquareBfd", &self.chi_square_bfd)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for FeatureFingerprint {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "header",
            "community_id",
            "communityId",
            "session_id",
            "sessionId",
            "ts",
            "is_encrypted",
            "isEncrypted",
            "tls_version",
            "tlsVersion",
            "ja3",
            "sni_hash",
            "sniHash",
            "cert_sha256",
            "certSha256",
            "cert_is_self_signed",
            "certIsSelfSigned",
            "pubkey_len",
            "pubkeyLen",
            "hex_freq",
            "hexFreq",
            "hex_ratio",
            "hexRatio",
            "entropy_payload",
            "entropyPayload",
            "chi_square_bfd",
            "chiSquareBfd",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Header,
            CommunityId,
            SessionId,
            Ts,
            IsEncrypted,
            TlsVersion,
            Ja3,
            SniHash,
            CertSha256,
            CertIsSelfSigned,
            PubkeyLen,
            HexFreq,
            HexRatio,
            EntropyPayload,
            ChiSquareBfd,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "header" => Ok(GeneratedField::Header),
                            "communityId" | "community_id" => Ok(GeneratedField::CommunityId),
                            "sessionId" | "session_id" => Ok(GeneratedField::SessionId),
                            "ts" => Ok(GeneratedField::Ts),
                            "isEncrypted" | "is_encrypted" => Ok(GeneratedField::IsEncrypted),
                            "tlsVersion" | "tls_version" => Ok(GeneratedField::TlsVersion),
                            "ja3" => Ok(GeneratedField::Ja3),
                            "sniHash" | "sni_hash" => Ok(GeneratedField::SniHash),
                            "certSha256" | "cert_sha256" => Ok(GeneratedField::CertSha256),
                            "certIsSelfSigned" | "cert_is_self_signed" => Ok(GeneratedField::CertIsSelfSigned),
                            "pubkeyLen" | "pubkey_len" => Ok(GeneratedField::PubkeyLen),
                            "hexFreq" | "hex_freq" => Ok(GeneratedField::HexFreq),
                            "hexRatio" | "hex_ratio" => Ok(GeneratedField::HexRatio),
                            "entropyPayload" | "entropy_payload" => Ok(GeneratedField::EntropyPayload),
                            "chiSquareBfd" | "chi_square_bfd" => Ok(GeneratedField::ChiSquareBfd),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = FeatureFingerprint;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.FeatureFingerprint")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<FeatureFingerprint, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut header__ = None;
                let mut community_id__ = None;
                let mut session_id__ = None;
                let mut ts__ = None;
                let mut is_encrypted__ = None;
                let mut tls_version__ = None;
                let mut ja3__ = None;
                let mut sni_hash__ = None;
                let mut cert_sha256__ = None;
                let mut cert_is_self_signed__ = None;
                let mut pubkey_len__ = None;
                let mut hex_freq__ = None;
                let mut hex_ratio__ = None;
                let mut entropy_payload__ = None;
                let mut chi_square_bfd__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Header => {
                            if header__.is_some() {
                                return Err(serde::de::Error::duplicate_field("header"));
                            }
                            header__ = map_.next_value()?;
                        }
                        GeneratedField::CommunityId => {
                            if community_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("communityId"));
                            }
                            community_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::SessionId => {
                            if session_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("sessionId"));
                            }
                            session_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Ts => {
                            if ts__.is_some() {
                                return Err(serde::de::Error::duplicate_field("ts"));
                            }
                            ts__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::IsEncrypted => {
                            if is_encrypted__.is_some() {
                                return Err(serde::de::Error::duplicate_field("isEncrypted"));
                            }
                            is_encrypted__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::TlsVersion => {
                            if tls_version__.is_some() {
                                return Err(serde::de::Error::duplicate_field("tlsVersion"));
                            }
                            tls_version__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Ja3 => {
                            if ja3__.is_some() {
                                return Err(serde::de::Error::duplicate_field("ja3"));
                            }
                            ja3__ = Some(map_.next_value()?);
                        }
                        GeneratedField::SniHash => {
                            if sni_hash__.is_some() {
                                return Err(serde::de::Error::duplicate_field("sniHash"));
                            }
                            sni_hash__ = Some(map_.next_value()?);
                        }
                        GeneratedField::CertSha256 => {
                            if cert_sha256__.is_some() {
                                return Err(serde::de::Error::duplicate_field("certSha256"));
                            }
                            cert_sha256__ = Some(map_.next_value()?);
                        }
                        GeneratedField::CertIsSelfSigned => {
                            if cert_is_self_signed__.is_some() {
                                return Err(serde::de::Error::duplicate_field("certIsSelfSigned"));
                            }
                            cert_is_self_signed__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::PubkeyLen => {
                            if pubkey_len__.is_some() {
                                return Err(serde::de::Error::duplicate_field("pubkeyLen"));
                            }
                            pubkey_len__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::HexFreq => {
                            if hex_freq__.is_some() {
                                return Err(serde::de::Error::duplicate_field("hexFreq"));
                            }
                            hex_freq__ = 
                                Some(map_.next_value::<Vec<::pbjson::private::NumberDeserialize<_>>>()?
                                    .into_iter().map(|x| x.0).collect())
                            ;
                        }
                        GeneratedField::HexRatio => {
                            if hex_ratio__.is_some() {
                                return Err(serde::de::Error::duplicate_field("hexRatio"));
                            }
                            hex_ratio__ = 
                                Some(map_.next_value::<Vec<::pbjson::private::NumberDeserialize<_>>>()?
                                    .into_iter().map(|x| x.0).collect())
                            ;
                        }
                        GeneratedField::EntropyPayload => {
                            if entropy_payload__.is_some() {
                                return Err(serde::de::Error::duplicate_field("entropyPayload"));
                            }
                            entropy_payload__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::ChiSquareBfd => {
                            if chi_square_bfd__.is_some() {
                                return Err(serde::de::Error::duplicate_field("chiSquareBfd"));
                            }
                            chi_square_bfd__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                    }
                }
                Ok(FeatureFingerprint {
                    header: header__,
                    community_id: community_id__.unwrap_or_default(),
                    session_id: session_id__.unwrap_or_default(),
                    ts: ts__.unwrap_or_default(),
                    is_encrypted: is_encrypted__.unwrap_or_default(),
                    tls_version: tls_version__.unwrap_or_default(),
                    ja3: ja3__.unwrap_or_default(),
                    sni_hash: sni_hash__.unwrap_or_default(),
                    cert_sha256: cert_sha256__.unwrap_or_default(),
                    cert_is_self_signed: cert_is_self_signed__.unwrap_or_default(),
                    pubkey_len: pubkey_len__.unwrap_or_default(),
                    hex_freq: hex_freq__.unwrap_or_default(),
                    hex_ratio: hex_ratio__.unwrap_or_default(),
                    entropy_payload: entropy_payload__.unwrap_or_default(),
                    chi_square_bfd: chi_square_bfd__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.FeatureFingerprint", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for FeatureSeq {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if self.header.is_some() {
            len += 1;
        }
        if !self.object_type.is_empty() {
            len += 1;
        }
        if !self.object_id.is_empty() {
            len += 1;
        }
        if !self.community_id.is_empty() {
            len += 1;
        }
        if !self.window_id.is_empty() {
            len += 1;
        }
        if self.ts_start != 0 {
            len += 1;
        }
        if self.ts_end != 0 {
            len += 1;
        }
        if !self.pktlen_seq_hash.is_empty() {
            len += 1;
        }
        if !self.iat_seq_hash.is_empty() {
            len += 1;
        }
        if self.wavelet_releng_fwd != 0. {
            len += 1;
        }
        if self.wavelet_releng_bwd != 0. {
            len += 1;
        }
        if self.wavelet_entropy_fwd != 0. {
            len += 1;
        }
        if self.wavelet_entropy_bwd != 0. {
            len += 1;
        }
        if self.wavelet_detail_mean_fwd != 0. {
            len += 1;
        }
        if self.wavelet_detail_mean_bwd != 0. {
            len += 1;
        }
        if self.wavelet_detail_std_fwd != 0. {
            len += 1;
        }
        if self.wavelet_detail_std_bwd != 0. {
            len += 1;
        }
        if !self.seq_blob_ref.is_empty() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.FeatureSeq", len)?;
        if let Some(v) = self.header.as_ref() {
            struct_ser.serialize_field("header", v)?;
        }
        if !self.object_type.is_empty() {
            struct_ser.serialize_field("objectType", &self.object_type)?;
        }
        if !self.object_id.is_empty() {
            struct_ser.serialize_field("objectId", &self.object_id)?;
        }
        if !self.community_id.is_empty() {
            struct_ser.serialize_field("communityId", &self.community_id)?;
        }
        if !self.window_id.is_empty() {
            struct_ser.serialize_field("windowId", &self.window_id)?;
        }
        if self.ts_start != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("tsStart", ToString::to_string(&self.ts_start).as_str())?;
        }
        if self.ts_end != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("tsEnd", ToString::to_string(&self.ts_end).as_str())?;
        }
        if !self.pktlen_seq_hash.is_empty() {
            struct_ser.serialize_field("pktlenSeqHash", &self.pktlen_seq_hash)?;
        }
        if !self.iat_seq_hash.is_empty() {
            struct_ser.serialize_field("iatSeqHash", &self.iat_seq_hash)?;
        }
        if self.wavelet_releng_fwd != 0. {
            struct_ser.serialize_field("waveletRelengFwd", &self.wavelet_releng_fwd)?;
        }
        if self.wavelet_releng_bwd != 0. {
            struct_ser.serialize_field("waveletRelengBwd", &self.wavelet_releng_bwd)?;
        }
        if self.wavelet_entropy_fwd != 0. {
            struct_ser.serialize_field("waveletEntropyFwd", &self.wavelet_entropy_fwd)?;
        }
        if self.wavelet_entropy_bwd != 0. {
            struct_ser.serialize_field("waveletEntropyBwd", &self.wavelet_entropy_bwd)?;
        }
        if self.wavelet_detail_mean_fwd != 0. {
            struct_ser.serialize_field("waveletDetailMeanFwd", &self.wavelet_detail_mean_fwd)?;
        }
        if self.wavelet_detail_mean_bwd != 0. {
            struct_ser.serialize_field("waveletDetailMeanBwd", &self.wavelet_detail_mean_bwd)?;
        }
        if self.wavelet_detail_std_fwd != 0. {
            struct_ser.serialize_field("waveletDetailStdFwd", &self.wavelet_detail_std_fwd)?;
        }
        if self.wavelet_detail_std_bwd != 0. {
            struct_ser.serialize_field("waveletDetailStdBwd", &self.wavelet_detail_std_bwd)?;
        }
        if !self.seq_blob_ref.is_empty() {
            struct_ser.serialize_field("seqBlobRef", &self.seq_blob_ref)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for FeatureSeq {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "header",
            "object_type",
            "objectType",
            "object_id",
            "objectId",
            "community_id",
            "communityId",
            "window_id",
            "windowId",
            "ts_start",
            "tsStart",
            "ts_end",
            "tsEnd",
            "pktlen_seq_hash",
            "pktlenSeqHash",
            "iat_seq_hash",
            "iatSeqHash",
            "wavelet_releng_fwd",
            "waveletRelengFwd",
            "wavelet_releng_bwd",
            "waveletRelengBwd",
            "wavelet_entropy_fwd",
            "waveletEntropyFwd",
            "wavelet_entropy_bwd",
            "waveletEntropyBwd",
            "wavelet_detail_mean_fwd",
            "waveletDetailMeanFwd",
            "wavelet_detail_mean_bwd",
            "waveletDetailMeanBwd",
            "wavelet_detail_std_fwd",
            "waveletDetailStdFwd",
            "wavelet_detail_std_bwd",
            "waveletDetailStdBwd",
            "seq_blob_ref",
            "seqBlobRef",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Header,
            ObjectType,
            ObjectId,
            CommunityId,
            WindowId,
            TsStart,
            TsEnd,
            PktlenSeqHash,
            IatSeqHash,
            WaveletRelengFwd,
            WaveletRelengBwd,
            WaveletEntropyFwd,
            WaveletEntropyBwd,
            WaveletDetailMeanFwd,
            WaveletDetailMeanBwd,
            WaveletDetailStdFwd,
            WaveletDetailStdBwd,
            SeqBlobRef,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "header" => Ok(GeneratedField::Header),
                            "objectType" | "object_type" => Ok(GeneratedField::ObjectType),
                            "objectId" | "object_id" => Ok(GeneratedField::ObjectId),
                            "communityId" | "community_id" => Ok(GeneratedField::CommunityId),
                            "windowId" | "window_id" => Ok(GeneratedField::WindowId),
                            "tsStart" | "ts_start" => Ok(GeneratedField::TsStart),
                            "tsEnd" | "ts_end" => Ok(GeneratedField::TsEnd),
                            "pktlenSeqHash" | "pktlen_seq_hash" => Ok(GeneratedField::PktlenSeqHash),
                            "iatSeqHash" | "iat_seq_hash" => Ok(GeneratedField::IatSeqHash),
                            "waveletRelengFwd" | "wavelet_releng_fwd" => Ok(GeneratedField::WaveletRelengFwd),
                            "waveletRelengBwd" | "wavelet_releng_bwd" => Ok(GeneratedField::WaveletRelengBwd),
                            "waveletEntropyFwd" | "wavelet_entropy_fwd" => Ok(GeneratedField::WaveletEntropyFwd),
                            "waveletEntropyBwd" | "wavelet_entropy_bwd" => Ok(GeneratedField::WaveletEntropyBwd),
                            "waveletDetailMeanFwd" | "wavelet_detail_mean_fwd" => Ok(GeneratedField::WaveletDetailMeanFwd),
                            "waveletDetailMeanBwd" | "wavelet_detail_mean_bwd" => Ok(GeneratedField::WaveletDetailMeanBwd),
                            "waveletDetailStdFwd" | "wavelet_detail_std_fwd" => Ok(GeneratedField::WaveletDetailStdFwd),
                            "waveletDetailStdBwd" | "wavelet_detail_std_bwd" => Ok(GeneratedField::WaveletDetailStdBwd),
                            "seqBlobRef" | "seq_blob_ref" => Ok(GeneratedField::SeqBlobRef),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = FeatureSeq;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.FeatureSeq")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<FeatureSeq, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut header__ = None;
                let mut object_type__ = None;
                let mut object_id__ = None;
                let mut community_id__ = None;
                let mut window_id__ = None;
                let mut ts_start__ = None;
                let mut ts_end__ = None;
                let mut pktlen_seq_hash__ = None;
                let mut iat_seq_hash__ = None;
                let mut wavelet_releng_fwd__ = None;
                let mut wavelet_releng_bwd__ = None;
                let mut wavelet_entropy_fwd__ = None;
                let mut wavelet_entropy_bwd__ = None;
                let mut wavelet_detail_mean_fwd__ = None;
                let mut wavelet_detail_mean_bwd__ = None;
                let mut wavelet_detail_std_fwd__ = None;
                let mut wavelet_detail_std_bwd__ = None;
                let mut seq_blob_ref__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Header => {
                            if header__.is_some() {
                                return Err(serde::de::Error::duplicate_field("header"));
                            }
                            header__ = map_.next_value()?;
                        }
                        GeneratedField::ObjectType => {
                            if object_type__.is_some() {
                                return Err(serde::de::Error::duplicate_field("objectType"));
                            }
                            object_type__ = Some(map_.next_value()?);
                        }
                        GeneratedField::ObjectId => {
                            if object_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("objectId"));
                            }
                            object_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::CommunityId => {
                            if community_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("communityId"));
                            }
                            community_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::WindowId => {
                            if window_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("windowId"));
                            }
                            window_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::TsStart => {
                            if ts_start__.is_some() {
                                return Err(serde::de::Error::duplicate_field("tsStart"));
                            }
                            ts_start__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::TsEnd => {
                            if ts_end__.is_some() {
                                return Err(serde::de::Error::duplicate_field("tsEnd"));
                            }
                            ts_end__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::PktlenSeqHash => {
                            if pktlen_seq_hash__.is_some() {
                                return Err(serde::de::Error::duplicate_field("pktlenSeqHash"));
                            }
                            pktlen_seq_hash__ = Some(map_.next_value()?);
                        }
                        GeneratedField::IatSeqHash => {
                            if iat_seq_hash__.is_some() {
                                return Err(serde::de::Error::duplicate_field("iatSeqHash"));
                            }
                            iat_seq_hash__ = Some(map_.next_value()?);
                        }
                        GeneratedField::WaveletRelengFwd => {
                            if wavelet_releng_fwd__.is_some() {
                                return Err(serde::de::Error::duplicate_field("waveletRelengFwd"));
                            }
                            wavelet_releng_fwd__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::WaveletRelengBwd => {
                            if wavelet_releng_bwd__.is_some() {
                                return Err(serde::de::Error::duplicate_field("waveletRelengBwd"));
                            }
                            wavelet_releng_bwd__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::WaveletEntropyFwd => {
                            if wavelet_entropy_fwd__.is_some() {
                                return Err(serde::de::Error::duplicate_field("waveletEntropyFwd"));
                            }
                            wavelet_entropy_fwd__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::WaveletEntropyBwd => {
                            if wavelet_entropy_bwd__.is_some() {
                                return Err(serde::de::Error::duplicate_field("waveletEntropyBwd"));
                            }
                            wavelet_entropy_bwd__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::WaveletDetailMeanFwd => {
                            if wavelet_detail_mean_fwd__.is_some() {
                                return Err(serde::de::Error::duplicate_field("waveletDetailMeanFwd"));
                            }
                            wavelet_detail_mean_fwd__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::WaveletDetailMeanBwd => {
                            if wavelet_detail_mean_bwd__.is_some() {
                                return Err(serde::de::Error::duplicate_field("waveletDetailMeanBwd"));
                            }
                            wavelet_detail_mean_bwd__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::WaveletDetailStdFwd => {
                            if wavelet_detail_std_fwd__.is_some() {
                                return Err(serde::de::Error::duplicate_field("waveletDetailStdFwd"));
                            }
                            wavelet_detail_std_fwd__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::WaveletDetailStdBwd => {
                            if wavelet_detail_std_bwd__.is_some() {
                                return Err(serde::de::Error::duplicate_field("waveletDetailStdBwd"));
                            }
                            wavelet_detail_std_bwd__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::SeqBlobRef => {
                            if seq_blob_ref__.is_some() {
                                return Err(serde::de::Error::duplicate_field("seqBlobRef"));
                            }
                            seq_blob_ref__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(FeatureSeq {
                    header: header__,
                    object_type: object_type__.unwrap_or_default(),
                    object_id: object_id__.unwrap_or_default(),
                    community_id: community_id__.unwrap_or_default(),
                    window_id: window_id__.unwrap_or_default(),
                    ts_start: ts_start__.unwrap_or_default(),
                    ts_end: ts_end__.unwrap_or_default(),
                    pktlen_seq_hash: pktlen_seq_hash__.unwrap_or_default(),
                    iat_seq_hash: iat_seq_hash__.unwrap_or_default(),
                    wavelet_releng_fwd: wavelet_releng_fwd__.unwrap_or_default(),
                    wavelet_releng_bwd: wavelet_releng_bwd__.unwrap_or_default(),
                    wavelet_entropy_fwd: wavelet_entropy_fwd__.unwrap_or_default(),
                    wavelet_entropy_bwd: wavelet_entropy_bwd__.unwrap_or_default(),
                    wavelet_detail_mean_fwd: wavelet_detail_mean_fwd__.unwrap_or_default(),
                    wavelet_detail_mean_bwd: wavelet_detail_mean_bwd__.unwrap_or_default(),
                    wavelet_detail_std_fwd: wavelet_detail_std_fwd__.unwrap_or_default(),
                    wavelet_detail_std_bwd: wavelet_detail_std_bwd__.unwrap_or_default(),
                    seq_blob_ref: seq_blob_ref__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.FeatureSeq", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for FeatureStat {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if self.header.is_some() {
            len += 1;
        }
        if !self.schema_version.is_empty() {
            len += 1;
        }
        if !self.object_type.is_empty() {
            len += 1;
        }
        if !self.object_id.is_empty() {
            len += 1;
        }
        if !self.community_id.is_empty() {
            len += 1;
        }
        if self.ts != 0 {
            len += 1;
        }
        if self.protocol != 0 {
            len += 1;
        }
        if self.duration_ms != 0 {
            len += 1;
        }
        if self.pps != 0. {
            len += 1;
        }
        if self.bps != 0. {
            len += 1;
        }
        if self.up_down_ratio != 0. {
            len += 1;
        }
        if self.pktlen_mean != 0. {
            len += 1;
        }
        if self.pktlen_std != 0. {
            len += 1;
        }
        if self.iat_mean_ms != 0. {
            len += 1;
        }
        if self.iat_std_ms != 0. {
            len += 1;
        }
        if self.active_mean_ms != 0. {
            len += 1;
        }
        if self.idle_mean_ms != 0. {
            len += 1;
        }
        if self.tcp_flag_syn_cnt != 0 {
            len += 1;
        }
        if self.tcp_flag_ack_cnt != 0 {
            len += 1;
        }
        if self.tcp_init_win_bytes_fwd != 0 {
            len += 1;
        }
        if self.tcp_init_win_bytes_bwd != 0 {
            len += 1;
        }
        if !self.extra.is_empty() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.FeatureStat", len)?;
        if let Some(v) = self.header.as_ref() {
            struct_ser.serialize_field("header", v)?;
        }
        if !self.schema_version.is_empty() {
            struct_ser.serialize_field("schemaVersion", &self.schema_version)?;
        }
        if !self.object_type.is_empty() {
            struct_ser.serialize_field("objectType", &self.object_type)?;
        }
        if !self.object_id.is_empty() {
            struct_ser.serialize_field("objectId", &self.object_id)?;
        }
        if !self.community_id.is_empty() {
            struct_ser.serialize_field("communityId", &self.community_id)?;
        }
        if self.ts != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("ts", ToString::to_string(&self.ts).as_str())?;
        }
        if self.protocol != 0 {
            struct_ser.serialize_field("protocol", &self.protocol)?;
        }
        if self.duration_ms != 0 {
            struct_ser.serialize_field("durationMs", &self.duration_ms)?;
        }
        if self.pps != 0. {
            struct_ser.serialize_field("pps", &self.pps)?;
        }
        if self.bps != 0. {
            struct_ser.serialize_field("bps", &self.bps)?;
        }
        if self.up_down_ratio != 0. {
            struct_ser.serialize_field("upDownRatio", &self.up_down_ratio)?;
        }
        if self.pktlen_mean != 0. {
            struct_ser.serialize_field("pktlenMean", &self.pktlen_mean)?;
        }
        if self.pktlen_std != 0. {
            struct_ser.serialize_field("pktlenStd", &self.pktlen_std)?;
        }
        if self.iat_mean_ms != 0. {
            struct_ser.serialize_field("iatMeanMs", &self.iat_mean_ms)?;
        }
        if self.iat_std_ms != 0. {
            struct_ser.serialize_field("iatStdMs", &self.iat_std_ms)?;
        }
        if self.active_mean_ms != 0. {
            struct_ser.serialize_field("activeMeanMs", &self.active_mean_ms)?;
        }
        if self.idle_mean_ms != 0. {
            struct_ser.serialize_field("idleMeanMs", &self.idle_mean_ms)?;
        }
        if self.tcp_flag_syn_cnt != 0 {
            struct_ser.serialize_field("tcpFlagSynCnt", &self.tcp_flag_syn_cnt)?;
        }
        if self.tcp_flag_ack_cnt != 0 {
            struct_ser.serialize_field("tcpFlagAckCnt", &self.tcp_flag_ack_cnt)?;
        }
        if self.tcp_init_win_bytes_fwd != 0 {
            struct_ser.serialize_field("tcpInitWinBytesFwd", &self.tcp_init_win_bytes_fwd)?;
        }
        if self.tcp_init_win_bytes_bwd != 0 {
            struct_ser.serialize_field("tcpInitWinBytesBwd", &self.tcp_init_win_bytes_bwd)?;
        }
        if !self.extra.is_empty() {
            struct_ser.serialize_field("extra", &self.extra)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for FeatureStat {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "header",
            "schema_version",
            "schemaVersion",
            "object_type",
            "objectType",
            "object_id",
            "objectId",
            "community_id",
            "communityId",
            "ts",
            "protocol",
            "duration_ms",
            "durationMs",
            "pps",
            "bps",
            "up_down_ratio",
            "upDownRatio",
            "pktlen_mean",
            "pktlenMean",
            "pktlen_std",
            "pktlenStd",
            "iat_mean_ms",
            "iatMeanMs",
            "iat_std_ms",
            "iatStdMs",
            "active_mean_ms",
            "activeMeanMs",
            "idle_mean_ms",
            "idleMeanMs",
            "tcp_flag_syn_cnt",
            "tcpFlagSynCnt",
            "tcp_flag_ack_cnt",
            "tcpFlagAckCnt",
            "tcp_init_win_bytes_fwd",
            "tcpInitWinBytesFwd",
            "tcp_init_win_bytes_bwd",
            "tcpInitWinBytesBwd",
            "extra",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Header,
            SchemaVersion,
            ObjectType,
            ObjectId,
            CommunityId,
            Ts,
            Protocol,
            DurationMs,
            Pps,
            Bps,
            UpDownRatio,
            PktlenMean,
            PktlenStd,
            IatMeanMs,
            IatStdMs,
            ActiveMeanMs,
            IdleMeanMs,
            TcpFlagSynCnt,
            TcpFlagAckCnt,
            TcpInitWinBytesFwd,
            TcpInitWinBytesBwd,
            Extra,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "header" => Ok(GeneratedField::Header),
                            "schemaVersion" | "schema_version" => Ok(GeneratedField::SchemaVersion),
                            "objectType" | "object_type" => Ok(GeneratedField::ObjectType),
                            "objectId" | "object_id" => Ok(GeneratedField::ObjectId),
                            "communityId" | "community_id" => Ok(GeneratedField::CommunityId),
                            "ts" => Ok(GeneratedField::Ts),
                            "protocol" => Ok(GeneratedField::Protocol),
                            "durationMs" | "duration_ms" => Ok(GeneratedField::DurationMs),
                            "pps" => Ok(GeneratedField::Pps),
                            "bps" => Ok(GeneratedField::Bps),
                            "upDownRatio" | "up_down_ratio" => Ok(GeneratedField::UpDownRatio),
                            "pktlenMean" | "pktlen_mean" => Ok(GeneratedField::PktlenMean),
                            "pktlenStd" | "pktlen_std" => Ok(GeneratedField::PktlenStd),
                            "iatMeanMs" | "iat_mean_ms" => Ok(GeneratedField::IatMeanMs),
                            "iatStdMs" | "iat_std_ms" => Ok(GeneratedField::IatStdMs),
                            "activeMeanMs" | "active_mean_ms" => Ok(GeneratedField::ActiveMeanMs),
                            "idleMeanMs" | "idle_mean_ms" => Ok(GeneratedField::IdleMeanMs),
                            "tcpFlagSynCnt" | "tcp_flag_syn_cnt" => Ok(GeneratedField::TcpFlagSynCnt),
                            "tcpFlagAckCnt" | "tcp_flag_ack_cnt" => Ok(GeneratedField::TcpFlagAckCnt),
                            "tcpInitWinBytesFwd" | "tcp_init_win_bytes_fwd" => Ok(GeneratedField::TcpInitWinBytesFwd),
                            "tcpInitWinBytesBwd" | "tcp_init_win_bytes_bwd" => Ok(GeneratedField::TcpInitWinBytesBwd),
                            "extra" => Ok(GeneratedField::Extra),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = FeatureStat;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.FeatureStat")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<FeatureStat, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut header__ = None;
                let mut schema_version__ = None;
                let mut object_type__ = None;
                let mut object_id__ = None;
                let mut community_id__ = None;
                let mut ts__ = None;
                let mut protocol__ = None;
                let mut duration_ms__ = None;
                let mut pps__ = None;
                let mut bps__ = None;
                let mut up_down_ratio__ = None;
                let mut pktlen_mean__ = None;
                let mut pktlen_std__ = None;
                let mut iat_mean_ms__ = None;
                let mut iat_std_ms__ = None;
                let mut active_mean_ms__ = None;
                let mut idle_mean_ms__ = None;
                let mut tcp_flag_syn_cnt__ = None;
                let mut tcp_flag_ack_cnt__ = None;
                let mut tcp_init_win_bytes_fwd__ = None;
                let mut tcp_init_win_bytes_bwd__ = None;
                let mut extra__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Header => {
                            if header__.is_some() {
                                return Err(serde::de::Error::duplicate_field("header"));
                            }
                            header__ = map_.next_value()?;
                        }
                        GeneratedField::SchemaVersion => {
                            if schema_version__.is_some() {
                                return Err(serde::de::Error::duplicate_field("schemaVersion"));
                            }
                            schema_version__ = Some(map_.next_value()?);
                        }
                        GeneratedField::ObjectType => {
                            if object_type__.is_some() {
                                return Err(serde::de::Error::duplicate_field("objectType"));
                            }
                            object_type__ = Some(map_.next_value()?);
                        }
                        GeneratedField::ObjectId => {
                            if object_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("objectId"));
                            }
                            object_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::CommunityId => {
                            if community_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("communityId"));
                            }
                            community_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Ts => {
                            if ts__.is_some() {
                                return Err(serde::de::Error::duplicate_field("ts"));
                            }
                            ts__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::Protocol => {
                            if protocol__.is_some() {
                                return Err(serde::de::Error::duplicate_field("protocol"));
                            }
                            protocol__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::DurationMs => {
                            if duration_ms__.is_some() {
                                return Err(serde::de::Error::duplicate_field("durationMs"));
                            }
                            duration_ms__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::Pps => {
                            if pps__.is_some() {
                                return Err(serde::de::Error::duplicate_field("pps"));
                            }
                            pps__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::Bps => {
                            if bps__.is_some() {
                                return Err(serde::de::Error::duplicate_field("bps"));
                            }
                            bps__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::UpDownRatio => {
                            if up_down_ratio__.is_some() {
                                return Err(serde::de::Error::duplicate_field("upDownRatio"));
                            }
                            up_down_ratio__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::PktlenMean => {
                            if pktlen_mean__.is_some() {
                                return Err(serde::de::Error::duplicate_field("pktlenMean"));
                            }
                            pktlen_mean__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::PktlenStd => {
                            if pktlen_std__.is_some() {
                                return Err(serde::de::Error::duplicate_field("pktlenStd"));
                            }
                            pktlen_std__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::IatMeanMs => {
                            if iat_mean_ms__.is_some() {
                                return Err(serde::de::Error::duplicate_field("iatMeanMs"));
                            }
                            iat_mean_ms__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::IatStdMs => {
                            if iat_std_ms__.is_some() {
                                return Err(serde::de::Error::duplicate_field("iatStdMs"));
                            }
                            iat_std_ms__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::ActiveMeanMs => {
                            if active_mean_ms__.is_some() {
                                return Err(serde::de::Error::duplicate_field("activeMeanMs"));
                            }
                            active_mean_ms__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::IdleMeanMs => {
                            if idle_mean_ms__.is_some() {
                                return Err(serde::de::Error::duplicate_field("idleMeanMs"));
                            }
                            idle_mean_ms__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::TcpFlagSynCnt => {
                            if tcp_flag_syn_cnt__.is_some() {
                                return Err(serde::de::Error::duplicate_field("tcpFlagSynCnt"));
                            }
                            tcp_flag_syn_cnt__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::TcpFlagAckCnt => {
                            if tcp_flag_ack_cnt__.is_some() {
                                return Err(serde::de::Error::duplicate_field("tcpFlagAckCnt"));
                            }
                            tcp_flag_ack_cnt__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::TcpInitWinBytesFwd => {
                            if tcp_init_win_bytes_fwd__.is_some() {
                                return Err(serde::de::Error::duplicate_field("tcpInitWinBytesFwd"));
                            }
                            tcp_init_win_bytes_fwd__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::TcpInitWinBytesBwd => {
                            if tcp_init_win_bytes_bwd__.is_some() {
                                return Err(serde::de::Error::duplicate_field("tcpInitWinBytesBwd"));
                            }
                            tcp_init_win_bytes_bwd__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::Extra => {
                            if extra__.is_some() {
                                return Err(serde::de::Error::duplicate_field("extra"));
                            }
                            extra__ = 
                                Some(map_.next_value::<Vec<::pbjson::private::NumberDeserialize<_>>>()?
                                    .into_iter().map(|x| x.0).collect())
                            ;
                        }
                    }
                }
                Ok(FeatureStat {
                    header: header__,
                    schema_version: schema_version__.unwrap_or_default(),
                    object_type: object_type__.unwrap_or_default(),
                    object_id: object_id__.unwrap_or_default(),
                    community_id: community_id__.unwrap_or_default(),
                    ts: ts__.unwrap_or_default(),
                    protocol: protocol__.unwrap_or_default(),
                    duration_ms: duration_ms__.unwrap_or_default(),
                    pps: pps__.unwrap_or_default(),
                    bps: bps__.unwrap_or_default(),
                    up_down_ratio: up_down_ratio__.unwrap_or_default(),
                    pktlen_mean: pktlen_mean__.unwrap_or_default(),
                    pktlen_std: pktlen_std__.unwrap_or_default(),
                    iat_mean_ms: iat_mean_ms__.unwrap_or_default(),
                    iat_std_ms: iat_std_ms__.unwrap_or_default(),
                    active_mean_ms: active_mean_ms__.unwrap_or_default(),
                    idle_mean_ms: idle_mean_ms__.unwrap_or_default(),
                    tcp_flag_syn_cnt: tcp_flag_syn_cnt__.unwrap_or_default(),
                    tcp_flag_ack_cnt: tcp_flag_ack_cnt__.unwrap_or_default(),
                    tcp_init_win_bytes_fwd: tcp_init_win_bytes_fwd__.unwrap_or_default(),
                    tcp_init_win_bytes_bwd: tcp_init_win_bytes_bwd__.unwrap_or_default(),
                    extra: extra__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.FeatureStat", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for FiveTuple {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.src_ip.is_empty() {
            len += 1;
        }
        if !self.dst_ip.is_empty() {
            len += 1;
        }
        if self.src_port != 0 {
            len += 1;
        }
        if self.dst_port != 0 {
            len += 1;
        }
        if self.protocol != 0 {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.FiveTuple", len)?;
        if !self.src_ip.is_empty() {
            struct_ser.serialize_field("srcIp", &self.src_ip)?;
        }
        if !self.dst_ip.is_empty() {
            struct_ser.serialize_field("dstIp", &self.dst_ip)?;
        }
        if self.src_port != 0 {
            struct_ser.serialize_field("srcPort", &self.src_port)?;
        }
        if self.dst_port != 0 {
            struct_ser.serialize_field("dstPort", &self.dst_port)?;
        }
        if self.protocol != 0 {
            struct_ser.serialize_field("protocol", &self.protocol)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for FiveTuple {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "src_ip",
            "srcIp",
            "dst_ip",
            "dstIp",
            "src_port",
            "srcPort",
            "dst_port",
            "dstPort",
            "protocol",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            SrcIp,
            DstIp,
            SrcPort,
            DstPort,
            Protocol,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "srcIp" | "src_ip" => Ok(GeneratedField::SrcIp),
                            "dstIp" | "dst_ip" => Ok(GeneratedField::DstIp),
                            "srcPort" | "src_port" => Ok(GeneratedField::SrcPort),
                            "dstPort" | "dst_port" => Ok(GeneratedField::DstPort),
                            "protocol" => Ok(GeneratedField::Protocol),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = FiveTuple;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.FiveTuple")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<FiveTuple, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut src_ip__ = None;
                let mut dst_ip__ = None;
                let mut src_port__ = None;
                let mut dst_port__ = None;
                let mut protocol__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::SrcIp => {
                            if src_ip__.is_some() {
                                return Err(serde::de::Error::duplicate_field("srcIp"));
                            }
                            src_ip__ = Some(map_.next_value()?);
                        }
                        GeneratedField::DstIp => {
                            if dst_ip__.is_some() {
                                return Err(serde::de::Error::duplicate_field("dstIp"));
                            }
                            dst_ip__ = Some(map_.next_value()?);
                        }
                        GeneratedField::SrcPort => {
                            if src_port__.is_some() {
                                return Err(serde::de::Error::duplicate_field("srcPort"));
                            }
                            src_port__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::DstPort => {
                            if dst_port__.is_some() {
                                return Err(serde::de::Error::duplicate_field("dstPort"));
                            }
                            dst_port__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::Protocol => {
                            if protocol__.is_some() {
                                return Err(serde::de::Error::duplicate_field("protocol"));
                            }
                            protocol__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                    }
                }
                Ok(FiveTuple {
                    src_ip: src_ip__.unwrap_or_default(),
                    dst_ip: dst_ip__.unwrap_or_default(),
                    src_port: src_port__.unwrap_or_default(),
                    dst_port: dst_port__.unwrap_or_default(),
                    protocol: protocol__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.FiveTuple", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for FlowBatch {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.flows.is_empty() {
            len += 1;
        }
        if self.metadata.is_some() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.FlowBatch", len)?;
        if !self.flows.is_empty() {
            struct_ser.serialize_field("flows", &self.flows)?;
        }
        if let Some(v) = self.metadata.as_ref() {
            struct_ser.serialize_field("metadata", v)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for FlowBatch {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "flows",
            "metadata",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Flows,
            Metadata,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "flows" => Ok(GeneratedField::Flows),
                            "metadata" => Ok(GeneratedField::Metadata),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = FlowBatch;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.FlowBatch")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<FlowBatch, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut flows__ = None;
                let mut metadata__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Flows => {
                            if flows__.is_some() {
                                return Err(serde::de::Error::duplicate_field("flows"));
                            }
                            flows__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Metadata => {
                            if metadata__.is_some() {
                                return Err(serde::de::Error::duplicate_field("metadata"));
                            }
                            metadata__ = map_.next_value()?;
                        }
                    }
                }
                Ok(FlowBatch {
                    flows: flows__.unwrap_or_default(),
                    metadata: metadata__,
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.FlowBatch", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for FlowDirection {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        let variant = match self {
            Self::Unspecified => "FLOW_DIRECTION_UNSPECIFIED",
            Self::Forward => "FLOW_DIRECTION_FORWARD",
            Self::Backward => "FLOW_DIRECTION_BACKWARD",
            Self::Bidirectional => "FLOW_DIRECTION_BIDIRECTIONAL",
        };
        serializer.serialize_str(variant)
    }
}
impl<'de> serde::Deserialize<'de> for FlowDirection {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "FLOW_DIRECTION_UNSPECIFIED",
            "FLOW_DIRECTION_FORWARD",
            "FLOW_DIRECTION_BACKWARD",
            "FLOW_DIRECTION_BIDIRECTIONAL",
        ];

        struct GeneratedVisitor;

        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = FlowDirection;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                write!(formatter, "expected one of: {:?}", &FIELDS)
            }

            fn visit_i64<E>(self, v: i64) -> std::result::Result<Self::Value, E>
            where
                E: serde::de::Error,
            {
                i32::try_from(v)
                    .ok()
                    .and_then(|x| x.try_into().ok())
                    .ok_or_else(|| {
                        serde::de::Error::invalid_value(serde::de::Unexpected::Signed(v), &self)
                    })
            }

            fn visit_u64<E>(self, v: u64) -> std::result::Result<Self::Value, E>
            where
                E: serde::de::Error,
            {
                i32::try_from(v)
                    .ok()
                    .and_then(|x| x.try_into().ok())
                    .ok_or_else(|| {
                        serde::de::Error::invalid_value(serde::de::Unexpected::Unsigned(v), &self)
                    })
            }

            fn visit_str<E>(self, value: &str) -> std::result::Result<Self::Value, E>
            where
                E: serde::de::Error,
            {
                match value {
                    "FLOW_DIRECTION_UNSPECIFIED" => Ok(FlowDirection::Unspecified),
                    "FLOW_DIRECTION_FORWARD" => Ok(FlowDirection::Forward),
                    "FLOW_DIRECTION_BACKWARD" => Ok(FlowDirection::Backward),
                    "FLOW_DIRECTION_BIDIRECTIONAL" => Ok(FlowDirection::Bidirectional),
                    _ => Err(serde::de::Error::unknown_variant(value, FIELDS)),
                }
            }
        }
        deserializer.deserialize_any(GeneratedVisitor)
    }
}
impl serde::Serialize for FlowEvent {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if self.header.is_some() {
            len += 1;
        }
        if !self.flow_id.is_empty() {
            len += 1;
        }
        if !self.community_id.is_empty() {
            len += 1;
        }
        if self.tuple.is_some() {
            len += 1;
        }
        if !self.direction.is_empty() {
            len += 1;
        }
        if self.ts_start != 0 {
            len += 1;
        }
        if self.ts_end != 0 {
            len += 1;
        }
        if self.duration_ms != 0 {
            len += 1;
        }
        if self.packets_fwd != 0 {
            len += 1;
        }
        if self.packets_bwd != 0 {
            len += 1;
        }
        if self.bytes_fwd != 0 {
            len += 1;
        }
        if self.bytes_bwd != 0 {
            len += 1;
        }
        if self.pps != 0. {
            len += 1;
        }
        if self.bps != 0. {
            len += 1;
        }
        if self.pktlen_stats.is_some() {
            len += 1;
        }
        if self.iat_stats.is_some() {
            len += 1;
        }
        if self.tcp_flags_fwd != 0 {
            len += 1;
        }
        if self.tcp_flags_bwd != 0 {
            len += 1;
        }
        if self.tos != 0 {
            len += 1;
        }
        if self.active_stats.is_some() {
            len += 1;
        }
        if self.idle_stats.is_some() {
            len += 1;
        }
        if self.subflow_count != 0 {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.FlowEvent", len)?;
        if let Some(v) = self.header.as_ref() {
            struct_ser.serialize_field("header", v)?;
        }
        if !self.flow_id.is_empty() {
            struct_ser.serialize_field("flowId", &self.flow_id)?;
        }
        if !self.community_id.is_empty() {
            struct_ser.serialize_field("communityId", &self.community_id)?;
        }
        if let Some(v) = self.tuple.as_ref() {
            struct_ser.serialize_field("tuple", v)?;
        }
        if !self.direction.is_empty() {
            struct_ser.serialize_field("direction", &self.direction)?;
        }
        if self.ts_start != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("tsStart", ToString::to_string(&self.ts_start).as_str())?;
        }
        if self.ts_end != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("tsEnd", ToString::to_string(&self.ts_end).as_str())?;
        }
        if self.duration_ms != 0 {
            struct_ser.serialize_field("durationMs", &self.duration_ms)?;
        }
        if self.packets_fwd != 0 {
            struct_ser.serialize_field("packetsFwd", &self.packets_fwd)?;
        }
        if self.packets_bwd != 0 {
            struct_ser.serialize_field("packetsBwd", &self.packets_bwd)?;
        }
        if self.bytes_fwd != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("bytesFwd", ToString::to_string(&self.bytes_fwd).as_str())?;
        }
        if self.bytes_bwd != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("bytesBwd", ToString::to_string(&self.bytes_bwd).as_str())?;
        }
        if self.pps != 0. {
            struct_ser.serialize_field("pps", &self.pps)?;
        }
        if self.bps != 0. {
            struct_ser.serialize_field("bps", &self.bps)?;
        }
        if let Some(v) = self.pktlen_stats.as_ref() {
            struct_ser.serialize_field("pktlenStats", v)?;
        }
        if let Some(v) = self.iat_stats.as_ref() {
            struct_ser.serialize_field("iatStats", v)?;
        }
        if self.tcp_flags_fwd != 0 {
            struct_ser.serialize_field("tcpFlagsFwd", &self.tcp_flags_fwd)?;
        }
        if self.tcp_flags_bwd != 0 {
            struct_ser.serialize_field("tcpFlagsBwd", &self.tcp_flags_bwd)?;
        }
        if self.tos != 0 {
            struct_ser.serialize_field("tos", &self.tos)?;
        }
        if let Some(v) = self.active_stats.as_ref() {
            struct_ser.serialize_field("activeStats", v)?;
        }
        if let Some(v) = self.idle_stats.as_ref() {
            struct_ser.serialize_field("idleStats", v)?;
        }
        if self.subflow_count != 0 {
            struct_ser.serialize_field("subflowCount", &self.subflow_count)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for FlowEvent {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "header",
            "flow_id",
            "flowId",
            "community_id",
            "communityId",
            "tuple",
            "direction",
            "ts_start",
            "tsStart",
            "ts_end",
            "tsEnd",
            "duration_ms",
            "durationMs",
            "packets_fwd",
            "packetsFwd",
            "packets_bwd",
            "packetsBwd",
            "bytes_fwd",
            "bytesFwd",
            "bytes_bwd",
            "bytesBwd",
            "pps",
            "bps",
            "pktlen_stats",
            "pktlenStats",
            "iat_stats",
            "iatStats",
            "tcp_flags_fwd",
            "tcpFlagsFwd",
            "tcp_flags_bwd",
            "tcpFlagsBwd",
            "tos",
            "active_stats",
            "activeStats",
            "idle_stats",
            "idleStats",
            "subflow_count",
            "subflowCount",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Header,
            FlowId,
            CommunityId,
            Tuple,
            Direction,
            TsStart,
            TsEnd,
            DurationMs,
            PacketsFwd,
            PacketsBwd,
            BytesFwd,
            BytesBwd,
            Pps,
            Bps,
            PktlenStats,
            IatStats,
            TcpFlagsFwd,
            TcpFlagsBwd,
            Tos,
            ActiveStats,
            IdleStats,
            SubflowCount,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "header" => Ok(GeneratedField::Header),
                            "flowId" | "flow_id" => Ok(GeneratedField::FlowId),
                            "communityId" | "community_id" => Ok(GeneratedField::CommunityId),
                            "tuple" => Ok(GeneratedField::Tuple),
                            "direction" => Ok(GeneratedField::Direction),
                            "tsStart" | "ts_start" => Ok(GeneratedField::TsStart),
                            "tsEnd" | "ts_end" => Ok(GeneratedField::TsEnd),
                            "durationMs" | "duration_ms" => Ok(GeneratedField::DurationMs),
                            "packetsFwd" | "packets_fwd" => Ok(GeneratedField::PacketsFwd),
                            "packetsBwd" | "packets_bwd" => Ok(GeneratedField::PacketsBwd),
                            "bytesFwd" | "bytes_fwd" => Ok(GeneratedField::BytesFwd),
                            "bytesBwd" | "bytes_bwd" => Ok(GeneratedField::BytesBwd),
                            "pps" => Ok(GeneratedField::Pps),
                            "bps" => Ok(GeneratedField::Bps),
                            "pktlenStats" | "pktlen_stats" => Ok(GeneratedField::PktlenStats),
                            "iatStats" | "iat_stats" => Ok(GeneratedField::IatStats),
                            "tcpFlagsFwd" | "tcp_flags_fwd" => Ok(GeneratedField::TcpFlagsFwd),
                            "tcpFlagsBwd" | "tcp_flags_bwd" => Ok(GeneratedField::TcpFlagsBwd),
                            "tos" => Ok(GeneratedField::Tos),
                            "activeStats" | "active_stats" => Ok(GeneratedField::ActiveStats),
                            "idleStats" | "idle_stats" => Ok(GeneratedField::IdleStats),
                            "subflowCount" | "subflow_count" => Ok(GeneratedField::SubflowCount),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = FlowEvent;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.FlowEvent")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<FlowEvent, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut header__ = None;
                let mut flow_id__ = None;
                let mut community_id__ = None;
                let mut tuple__ = None;
                let mut direction__ = None;
                let mut ts_start__ = None;
                let mut ts_end__ = None;
                let mut duration_ms__ = None;
                let mut packets_fwd__ = None;
                let mut packets_bwd__ = None;
                let mut bytes_fwd__ = None;
                let mut bytes_bwd__ = None;
                let mut pps__ = None;
                let mut bps__ = None;
                let mut pktlen_stats__ = None;
                let mut iat_stats__ = None;
                let mut tcp_flags_fwd__ = None;
                let mut tcp_flags_bwd__ = None;
                let mut tos__ = None;
                let mut active_stats__ = None;
                let mut idle_stats__ = None;
                let mut subflow_count__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Header => {
                            if header__.is_some() {
                                return Err(serde::de::Error::duplicate_field("header"));
                            }
                            header__ = map_.next_value()?;
                        }
                        GeneratedField::FlowId => {
                            if flow_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("flowId"));
                            }
                            flow_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::CommunityId => {
                            if community_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("communityId"));
                            }
                            community_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Tuple => {
                            if tuple__.is_some() {
                                return Err(serde::de::Error::duplicate_field("tuple"));
                            }
                            tuple__ = map_.next_value()?;
                        }
                        GeneratedField::Direction => {
                            if direction__.is_some() {
                                return Err(serde::de::Error::duplicate_field("direction"));
                            }
                            direction__ = Some(map_.next_value()?);
                        }
                        GeneratedField::TsStart => {
                            if ts_start__.is_some() {
                                return Err(serde::de::Error::duplicate_field("tsStart"));
                            }
                            ts_start__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::TsEnd => {
                            if ts_end__.is_some() {
                                return Err(serde::de::Error::duplicate_field("tsEnd"));
                            }
                            ts_end__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::DurationMs => {
                            if duration_ms__.is_some() {
                                return Err(serde::de::Error::duplicate_field("durationMs"));
                            }
                            duration_ms__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::PacketsFwd => {
                            if packets_fwd__.is_some() {
                                return Err(serde::de::Error::duplicate_field("packetsFwd"));
                            }
                            packets_fwd__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::PacketsBwd => {
                            if packets_bwd__.is_some() {
                                return Err(serde::de::Error::duplicate_field("packetsBwd"));
                            }
                            packets_bwd__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::BytesFwd => {
                            if bytes_fwd__.is_some() {
                                return Err(serde::de::Error::duplicate_field("bytesFwd"));
                            }
                            bytes_fwd__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::BytesBwd => {
                            if bytes_bwd__.is_some() {
                                return Err(serde::de::Error::duplicate_field("bytesBwd"));
                            }
                            bytes_bwd__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::Pps => {
                            if pps__.is_some() {
                                return Err(serde::de::Error::duplicate_field("pps"));
                            }
                            pps__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::Bps => {
                            if bps__.is_some() {
                                return Err(serde::de::Error::duplicate_field("bps"));
                            }
                            bps__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::PktlenStats => {
                            if pktlen_stats__.is_some() {
                                return Err(serde::de::Error::duplicate_field("pktlenStats"));
                            }
                            pktlen_stats__ = map_.next_value()?;
                        }
                        GeneratedField::IatStats => {
                            if iat_stats__.is_some() {
                                return Err(serde::de::Error::duplicate_field("iatStats"));
                            }
                            iat_stats__ = map_.next_value()?;
                        }
                        GeneratedField::TcpFlagsFwd => {
                            if tcp_flags_fwd__.is_some() {
                                return Err(serde::de::Error::duplicate_field("tcpFlagsFwd"));
                            }
                            tcp_flags_fwd__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::TcpFlagsBwd => {
                            if tcp_flags_bwd__.is_some() {
                                return Err(serde::de::Error::duplicate_field("tcpFlagsBwd"));
                            }
                            tcp_flags_bwd__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::Tos => {
                            if tos__.is_some() {
                                return Err(serde::de::Error::duplicate_field("tos"));
                            }
                            tos__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::ActiveStats => {
                            if active_stats__.is_some() {
                                return Err(serde::de::Error::duplicate_field("activeStats"));
                            }
                            active_stats__ = map_.next_value()?;
                        }
                        GeneratedField::IdleStats => {
                            if idle_stats__.is_some() {
                                return Err(serde::de::Error::duplicate_field("idleStats"));
                            }
                            idle_stats__ = map_.next_value()?;
                        }
                        GeneratedField::SubflowCount => {
                            if subflow_count__.is_some() {
                                return Err(serde::de::Error::duplicate_field("subflowCount"));
                            }
                            subflow_count__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                    }
                }
                Ok(FlowEvent {
                    header: header__,
                    flow_id: flow_id__.unwrap_or_default(),
                    community_id: community_id__.unwrap_or_default(),
                    tuple: tuple__,
                    direction: direction__.unwrap_or_default(),
                    ts_start: ts_start__.unwrap_or_default(),
                    ts_end: ts_end__.unwrap_or_default(),
                    duration_ms: duration_ms__.unwrap_or_default(),
                    packets_fwd: packets_fwd__.unwrap_or_default(),
                    packets_bwd: packets_bwd__.unwrap_or_default(),
                    bytes_fwd: bytes_fwd__.unwrap_or_default(),
                    bytes_bwd: bytes_bwd__.unwrap_or_default(),
                    pps: pps__.unwrap_or_default(),
                    bps: bps__.unwrap_or_default(),
                    pktlen_stats: pktlen_stats__,
                    iat_stats: iat_stats__,
                    tcp_flags_fwd: tcp_flags_fwd__.unwrap_or_default(),
                    tcp_flags_bwd: tcp_flags_bwd__.unwrap_or_default(),
                    tos: tos__.unwrap_or_default(),
                    active_stats: active_stats__,
                    idle_stats: idle_stats__,
                    subflow_count: subflow_count__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.FlowEvent", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for GetAssetHistoryRequest {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.asset_id.is_empty() {
            len += 1;
        }
        if !self.tenant_id.is_empty() {
            len += 1;
        }
        if self.page_size != 0 {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.GetAssetHistoryRequest", len)?;
        if !self.asset_id.is_empty() {
            struct_ser.serialize_field("assetId", &self.asset_id)?;
        }
        if !self.tenant_id.is_empty() {
            struct_ser.serialize_field("tenantId", &self.tenant_id)?;
        }
        if self.page_size != 0 {
            struct_ser.serialize_field("pageSize", &self.page_size)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for GetAssetHistoryRequest {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "asset_id",
            "assetId",
            "tenant_id",
            "tenantId",
            "page_size",
            "pageSize",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            AssetId,
            TenantId,
            PageSize,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "assetId" | "asset_id" => Ok(GeneratedField::AssetId),
                            "tenantId" | "tenant_id" => Ok(GeneratedField::TenantId),
                            "pageSize" | "page_size" => Ok(GeneratedField::PageSize),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = GetAssetHistoryRequest;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.GetAssetHistoryRequest")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<GetAssetHistoryRequest, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut asset_id__ = None;
                let mut tenant_id__ = None;
                let mut page_size__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::AssetId => {
                            if asset_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("assetId"));
                            }
                            asset_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::TenantId => {
                            if tenant_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("tenantId"));
                            }
                            tenant_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::PageSize => {
                            if page_size__.is_some() {
                                return Err(serde::de::Error::duplicate_field("pageSize"));
                            }
                            page_size__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                    }
                }
                Ok(GetAssetHistoryRequest {
                    asset_id: asset_id__.unwrap_or_default(),
                    tenant_id: tenant_id__.unwrap_or_default(),
                    page_size: page_size__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.GetAssetHistoryRequest", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for GetAssetHistoryResponse {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.events.is_empty() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.GetAssetHistoryResponse", len)?;
        if !self.events.is_empty() {
            struct_ser.serialize_field("events", &self.events)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for GetAssetHistoryResponse {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "events",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Events,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "events" => Ok(GeneratedField::Events),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = GetAssetHistoryResponse;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.GetAssetHistoryResponse")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<GetAssetHistoryResponse, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut events__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Events => {
                            if events__.is_some() {
                                return Err(serde::de::Error::duplicate_field("events"));
                            }
                            events__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(GetAssetHistoryResponse {
                    events: events__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.GetAssetHistoryResponse", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for GetAssetRequest {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.asset_id.is_empty() {
            len += 1;
        }
        if !self.mac_address.is_empty() {
            len += 1;
        }
        if !self.tenant_id.is_empty() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.GetAssetRequest", len)?;
        if !self.asset_id.is_empty() {
            struct_ser.serialize_field("assetId", &self.asset_id)?;
        }
        if !self.mac_address.is_empty() {
            struct_ser.serialize_field("macAddress", &self.mac_address)?;
        }
        if !self.tenant_id.is_empty() {
            struct_ser.serialize_field("tenantId", &self.tenant_id)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for GetAssetRequest {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "asset_id",
            "assetId",
            "mac_address",
            "macAddress",
            "tenant_id",
            "tenantId",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            AssetId,
            MacAddress,
            TenantId,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "assetId" | "asset_id" => Ok(GeneratedField::AssetId),
                            "macAddress" | "mac_address" => Ok(GeneratedField::MacAddress),
                            "tenantId" | "tenant_id" => Ok(GeneratedField::TenantId),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = GetAssetRequest;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.GetAssetRequest")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<GetAssetRequest, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut asset_id__ = None;
                let mut mac_address__ = None;
                let mut tenant_id__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::AssetId => {
                            if asset_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("assetId"));
                            }
                            asset_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::MacAddress => {
                            if mac_address__.is_some() {
                                return Err(serde::de::Error::duplicate_field("macAddress"));
                            }
                            mac_address__ = Some(map_.next_value()?);
                        }
                        GeneratedField::TenantId => {
                            if tenant_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("tenantId"));
                            }
                            tenant_id__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(GetAssetRequest {
                    asset_id: asset_id__.unwrap_or_default(),
                    mac_address: mac_address__.unwrap_or_default(),
                    tenant_id: tenant_id__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.GetAssetRequest", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for GetAssetResponse {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if self.asset.is_some() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.GetAssetResponse", len)?;
        if let Some(v) = self.asset.as_ref() {
            struct_ser.serialize_field("asset", v)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for GetAssetResponse {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "asset",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Asset,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "asset" => Ok(GeneratedField::Asset),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = GetAssetResponse;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.GetAssetResponse")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<GetAssetResponse, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut asset__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Asset => {
                            if asset__.is_some() {
                                return Err(serde::de::Error::duplicate_field("asset"));
                            }
                            asset__ = map_.next_value()?;
                        }
                    }
                }
                Ok(GetAssetResponse {
                    asset: asset__,
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.GetAssetResponse", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for GraphCacheStats {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.tenant_id.is_empty() {
            len += 1;
        }
        if self.hour != 0 {
            len += 1;
        }
        if !self.query_type.is_empty() {
            len += 1;
        }
        if self.total_queries != 0 {
            len += 1;
        }
        if self.cache_hits != 0 {
            len += 1;
        }
        if self.cache_misses != 0 {
            len += 1;
        }
        if self.avg_duration_ms != 0. {
            len += 1;
        }
        if self.p95_duration_ms != 0. {
            len += 1;
        }
        if self.p99_duration_ms != 0. {
            len += 1;
        }
        if self.total_nodes != 0 {
            len += 1;
        }
        if self.total_edges != 0 {
            len += 1;
        }
        if self.error_count != 0 {
            len += 1;
        }
        if self.timeout_count != 0 {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.GraphCacheStats", len)?;
        if !self.tenant_id.is_empty() {
            struct_ser.serialize_field("tenantId", &self.tenant_id)?;
        }
        if self.hour != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("hour", ToString::to_string(&self.hour).as_str())?;
        }
        if !self.query_type.is_empty() {
            struct_ser.serialize_field("queryType", &self.query_type)?;
        }
        if self.total_queries != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("totalQueries", ToString::to_string(&self.total_queries).as_str())?;
        }
        if self.cache_hits != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("cacheHits", ToString::to_string(&self.cache_hits).as_str())?;
        }
        if self.cache_misses != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("cacheMisses", ToString::to_string(&self.cache_misses).as_str())?;
        }
        if self.avg_duration_ms != 0. {
            struct_ser.serialize_field("avgDurationMs", &self.avg_duration_ms)?;
        }
        if self.p95_duration_ms != 0. {
            struct_ser.serialize_field("p95DurationMs", &self.p95_duration_ms)?;
        }
        if self.p99_duration_ms != 0. {
            struct_ser.serialize_field("p99DurationMs", &self.p99_duration_ms)?;
        }
        if self.total_nodes != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("totalNodes", ToString::to_string(&self.total_nodes).as_str())?;
        }
        if self.total_edges != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("totalEdges", ToString::to_string(&self.total_edges).as_str())?;
        }
        if self.error_count != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("errorCount", ToString::to_string(&self.error_count).as_str())?;
        }
        if self.timeout_count != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("timeoutCount", ToString::to_string(&self.timeout_count).as_str())?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for GraphCacheStats {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "tenant_id",
            "tenantId",
            "hour",
            "query_type",
            "queryType",
            "total_queries",
            "totalQueries",
            "cache_hits",
            "cacheHits",
            "cache_misses",
            "cacheMisses",
            "avg_duration_ms",
            "avgDurationMs",
            "p95_duration_ms",
            "p95DurationMs",
            "p99_duration_ms",
            "p99DurationMs",
            "total_nodes",
            "totalNodes",
            "total_edges",
            "totalEdges",
            "error_count",
            "errorCount",
            "timeout_count",
            "timeoutCount",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            TenantId,
            Hour,
            QueryType,
            TotalQueries,
            CacheHits,
            CacheMisses,
            AvgDurationMs,
            P95DurationMs,
            P99DurationMs,
            TotalNodes,
            TotalEdges,
            ErrorCount,
            TimeoutCount,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "tenantId" | "tenant_id" => Ok(GeneratedField::TenantId),
                            "hour" => Ok(GeneratedField::Hour),
                            "queryType" | "query_type" => Ok(GeneratedField::QueryType),
                            "totalQueries" | "total_queries" => Ok(GeneratedField::TotalQueries),
                            "cacheHits" | "cache_hits" => Ok(GeneratedField::CacheHits),
                            "cacheMisses" | "cache_misses" => Ok(GeneratedField::CacheMisses),
                            "avgDurationMs" | "avg_duration_ms" => Ok(GeneratedField::AvgDurationMs),
                            "p95DurationMs" | "p95_duration_ms" => Ok(GeneratedField::P95DurationMs),
                            "p99DurationMs" | "p99_duration_ms" => Ok(GeneratedField::P99DurationMs),
                            "totalNodes" | "total_nodes" => Ok(GeneratedField::TotalNodes),
                            "totalEdges" | "total_edges" => Ok(GeneratedField::TotalEdges),
                            "errorCount" | "error_count" => Ok(GeneratedField::ErrorCount),
                            "timeoutCount" | "timeout_count" => Ok(GeneratedField::TimeoutCount),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = GraphCacheStats;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.GraphCacheStats")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<GraphCacheStats, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut tenant_id__ = None;
                let mut hour__ = None;
                let mut query_type__ = None;
                let mut total_queries__ = None;
                let mut cache_hits__ = None;
                let mut cache_misses__ = None;
                let mut avg_duration_ms__ = None;
                let mut p95_duration_ms__ = None;
                let mut p99_duration_ms__ = None;
                let mut total_nodes__ = None;
                let mut total_edges__ = None;
                let mut error_count__ = None;
                let mut timeout_count__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::TenantId => {
                            if tenant_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("tenantId"));
                            }
                            tenant_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Hour => {
                            if hour__.is_some() {
                                return Err(serde::de::Error::duplicate_field("hour"));
                            }
                            hour__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::QueryType => {
                            if query_type__.is_some() {
                                return Err(serde::de::Error::duplicate_field("queryType"));
                            }
                            query_type__ = Some(map_.next_value()?);
                        }
                        GeneratedField::TotalQueries => {
                            if total_queries__.is_some() {
                                return Err(serde::de::Error::duplicate_field("totalQueries"));
                            }
                            total_queries__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::CacheHits => {
                            if cache_hits__.is_some() {
                                return Err(serde::de::Error::duplicate_field("cacheHits"));
                            }
                            cache_hits__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::CacheMisses => {
                            if cache_misses__.is_some() {
                                return Err(serde::de::Error::duplicate_field("cacheMisses"));
                            }
                            cache_misses__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::AvgDurationMs => {
                            if avg_duration_ms__.is_some() {
                                return Err(serde::de::Error::duplicate_field("avgDurationMs"));
                            }
                            avg_duration_ms__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::P95DurationMs => {
                            if p95_duration_ms__.is_some() {
                                return Err(serde::de::Error::duplicate_field("p95DurationMs"));
                            }
                            p95_duration_ms__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::P99DurationMs => {
                            if p99_duration_ms__.is_some() {
                                return Err(serde::de::Error::duplicate_field("p99DurationMs"));
                            }
                            p99_duration_ms__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::TotalNodes => {
                            if total_nodes__.is_some() {
                                return Err(serde::de::Error::duplicate_field("totalNodes"));
                            }
                            total_nodes__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::TotalEdges => {
                            if total_edges__.is_some() {
                                return Err(serde::de::Error::duplicate_field("totalEdges"));
                            }
                            total_edges__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::ErrorCount => {
                            if error_count__.is_some() {
                                return Err(serde::de::Error::duplicate_field("errorCount"));
                            }
                            error_count__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::TimeoutCount => {
                            if timeout_count__.is_some() {
                                return Err(serde::de::Error::duplicate_field("timeoutCount"));
                            }
                            timeout_count__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                    }
                }
                Ok(GraphCacheStats {
                    tenant_id: tenant_id__.unwrap_or_default(),
                    hour: hour__.unwrap_or_default(),
                    query_type: query_type__.unwrap_or_default(),
                    total_queries: total_queries__.unwrap_or_default(),
                    cache_hits: cache_hits__.unwrap_or_default(),
                    cache_misses: cache_misses__.unwrap_or_default(),
                    avg_duration_ms: avg_duration_ms__.unwrap_or_default(),
                    p95_duration_ms: p95_duration_ms__.unwrap_or_default(),
                    p99_duration_ms: p99_duration_ms__.unwrap_or_default(),
                    total_nodes: total_nodes__.unwrap_or_default(),
                    total_edges: total_edges__.unwrap_or_default(),
                    error_count: error_count__.unwrap_or_default(),
                    timeout_count: timeout_count__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.GraphCacheStats", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for GraphHotIp {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.tenant_id.is_empty() {
            len += 1;
        }
        if self.date != 0 {
            len += 1;
        }
        if !self.ip.is_empty() {
            len += 1;
        }
        if self.query_count != 0 {
            len += 1;
        }
        if self.total_neighbors != 0 {
            len += 1;
        }
        if self.avg_session_count != 0. {
            len += 1;
        }
        if self.last_query_time != 0 {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.GraphHotIP", len)?;
        if !self.tenant_id.is_empty() {
            struct_ser.serialize_field("tenantId", &self.tenant_id)?;
        }
        if self.date != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("date", ToString::to_string(&self.date).as_str())?;
        }
        if !self.ip.is_empty() {
            struct_ser.serialize_field("ip", &self.ip)?;
        }
        if self.query_count != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("queryCount", ToString::to_string(&self.query_count).as_str())?;
        }
        if self.total_neighbors != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("totalNeighbors", ToString::to_string(&self.total_neighbors).as_str())?;
        }
        if self.avg_session_count != 0. {
            struct_ser.serialize_field("avgSessionCount", &self.avg_session_count)?;
        }
        if self.last_query_time != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("lastQueryTime", ToString::to_string(&self.last_query_time).as_str())?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for GraphHotIp {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "tenant_id",
            "tenantId",
            "date",
            "ip",
            "query_count",
            "queryCount",
            "total_neighbors",
            "totalNeighbors",
            "avg_session_count",
            "avgSessionCount",
            "last_query_time",
            "lastQueryTime",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            TenantId,
            Date,
            Ip,
            QueryCount,
            TotalNeighbors,
            AvgSessionCount,
            LastQueryTime,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "tenantId" | "tenant_id" => Ok(GeneratedField::TenantId),
                            "date" => Ok(GeneratedField::Date),
                            "ip" => Ok(GeneratedField::Ip),
                            "queryCount" | "query_count" => Ok(GeneratedField::QueryCount),
                            "totalNeighbors" | "total_neighbors" => Ok(GeneratedField::TotalNeighbors),
                            "avgSessionCount" | "avg_session_count" => Ok(GeneratedField::AvgSessionCount),
                            "lastQueryTime" | "last_query_time" => Ok(GeneratedField::LastQueryTime),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = GraphHotIp;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.GraphHotIP")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<GraphHotIp, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut tenant_id__ = None;
                let mut date__ = None;
                let mut ip__ = None;
                let mut query_count__ = None;
                let mut total_neighbors__ = None;
                let mut avg_session_count__ = None;
                let mut last_query_time__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::TenantId => {
                            if tenant_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("tenantId"));
                            }
                            tenant_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Date => {
                            if date__.is_some() {
                                return Err(serde::de::Error::duplicate_field("date"));
                            }
                            date__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::Ip => {
                            if ip__.is_some() {
                                return Err(serde::de::Error::duplicate_field("ip"));
                            }
                            ip__ = Some(map_.next_value()?);
                        }
                        GeneratedField::QueryCount => {
                            if query_count__.is_some() {
                                return Err(serde::de::Error::duplicate_field("queryCount"));
                            }
                            query_count__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::TotalNeighbors => {
                            if total_neighbors__.is_some() {
                                return Err(serde::de::Error::duplicate_field("totalNeighbors"));
                            }
                            total_neighbors__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::AvgSessionCount => {
                            if avg_session_count__.is_some() {
                                return Err(serde::de::Error::duplicate_field("avgSessionCount"));
                            }
                            avg_session_count__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::LastQueryTime => {
                            if last_query_time__.is_some() {
                                return Err(serde::de::Error::duplicate_field("lastQueryTime"));
                            }
                            last_query_time__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                    }
                }
                Ok(GraphHotIp {
                    tenant_id: tenant_id__.unwrap_or_default(),
                    date: date__.unwrap_or_default(),
                    ip: ip__.unwrap_or_default(),
                    query_count: query_count__.unwrap_or_default(),
                    total_neighbors: total_neighbors__.unwrap_or_default(),
                    avg_session_count: avg_session_count__.unwrap_or_default(),
                    last_query_time: last_query_time__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.GraphHotIP", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for GraphIpAffinity {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.tenant_id.is_empty() {
            len += 1;
        }
        if self.date != 0 {
            len += 1;
        }
        if !self.ip_a.is_empty() {
            len += 1;
        }
        if !self.ip_b.is_empty() {
            len += 1;
        }
        if self.session_count != 0 {
            len += 1;
        }
        if self.total_bytes != 0 {
            len += 1;
        }
        if self.avg_duration_ms != 0. {
            len += 1;
        }
        if self.a_to_b_count != 0 {
            len += 1;
        }
        if self.b_to_a_count != 0 {
            len += 1;
        }
        if self.first_seen != 0 {
            len += 1;
        }
        if self.last_seen != 0 {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.GraphIPAffinity", len)?;
        if !self.tenant_id.is_empty() {
            struct_ser.serialize_field("tenantId", &self.tenant_id)?;
        }
        if self.date != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("date", ToString::to_string(&self.date).as_str())?;
        }
        if !self.ip_a.is_empty() {
            struct_ser.serialize_field("ipA", &self.ip_a)?;
        }
        if !self.ip_b.is_empty() {
            struct_ser.serialize_field("ipB", &self.ip_b)?;
        }
        if self.session_count != 0 {
            struct_ser.serialize_field("sessionCount", &self.session_count)?;
        }
        if self.total_bytes != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("totalBytes", ToString::to_string(&self.total_bytes).as_str())?;
        }
        if self.avg_duration_ms != 0. {
            struct_ser.serialize_field("avgDurationMs", &self.avg_duration_ms)?;
        }
        if self.a_to_b_count != 0 {
            struct_ser.serialize_field("aToBCount", &self.a_to_b_count)?;
        }
        if self.b_to_a_count != 0 {
            struct_ser.serialize_field("bToACount", &self.b_to_a_count)?;
        }
        if self.first_seen != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("firstSeen", ToString::to_string(&self.first_seen).as_str())?;
        }
        if self.last_seen != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("lastSeen", ToString::to_string(&self.last_seen).as_str())?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for GraphIpAffinity {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "tenant_id",
            "tenantId",
            "date",
            "ip_a",
            "ipA",
            "ip_b",
            "ipB",
            "session_count",
            "sessionCount",
            "total_bytes",
            "totalBytes",
            "avg_duration_ms",
            "avgDurationMs",
            "a_to_b_count",
            "aToBCount",
            "b_to_a_count",
            "bToACount",
            "first_seen",
            "firstSeen",
            "last_seen",
            "lastSeen",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            TenantId,
            Date,
            IpA,
            IpB,
            SessionCount,
            TotalBytes,
            AvgDurationMs,
            AToBCount,
            BToACount,
            FirstSeen,
            LastSeen,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "tenantId" | "tenant_id" => Ok(GeneratedField::TenantId),
                            "date" => Ok(GeneratedField::Date),
                            "ipA" | "ip_a" => Ok(GeneratedField::IpA),
                            "ipB" | "ip_b" => Ok(GeneratedField::IpB),
                            "sessionCount" | "session_count" => Ok(GeneratedField::SessionCount),
                            "totalBytes" | "total_bytes" => Ok(GeneratedField::TotalBytes),
                            "avgDurationMs" | "avg_duration_ms" => Ok(GeneratedField::AvgDurationMs),
                            "aToBCount" | "a_to_b_count" => Ok(GeneratedField::AToBCount),
                            "bToACount" | "b_to_a_count" => Ok(GeneratedField::BToACount),
                            "firstSeen" | "first_seen" => Ok(GeneratedField::FirstSeen),
                            "lastSeen" | "last_seen" => Ok(GeneratedField::LastSeen),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = GraphIpAffinity;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.GraphIPAffinity")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<GraphIpAffinity, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut tenant_id__ = None;
                let mut date__ = None;
                let mut ip_a__ = None;
                let mut ip_b__ = None;
                let mut session_count__ = None;
                let mut total_bytes__ = None;
                let mut avg_duration_ms__ = None;
                let mut a_to_b_count__ = None;
                let mut b_to_a_count__ = None;
                let mut first_seen__ = None;
                let mut last_seen__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::TenantId => {
                            if tenant_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("tenantId"));
                            }
                            tenant_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Date => {
                            if date__.is_some() {
                                return Err(serde::de::Error::duplicate_field("date"));
                            }
                            date__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::IpA => {
                            if ip_a__.is_some() {
                                return Err(serde::de::Error::duplicate_field("ipA"));
                            }
                            ip_a__ = Some(map_.next_value()?);
                        }
                        GeneratedField::IpB => {
                            if ip_b__.is_some() {
                                return Err(serde::de::Error::duplicate_field("ipB"));
                            }
                            ip_b__ = Some(map_.next_value()?);
                        }
                        GeneratedField::SessionCount => {
                            if session_count__.is_some() {
                                return Err(serde::de::Error::duplicate_field("sessionCount"));
                            }
                            session_count__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::TotalBytes => {
                            if total_bytes__.is_some() {
                                return Err(serde::de::Error::duplicate_field("totalBytes"));
                            }
                            total_bytes__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::AvgDurationMs => {
                            if avg_duration_ms__.is_some() {
                                return Err(serde::de::Error::duplicate_field("avgDurationMs"));
                            }
                            avg_duration_ms__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::AToBCount => {
                            if a_to_b_count__.is_some() {
                                return Err(serde::de::Error::duplicate_field("aToBCount"));
                            }
                            a_to_b_count__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::BToACount => {
                            if b_to_a_count__.is_some() {
                                return Err(serde::de::Error::duplicate_field("bToACount"));
                            }
                            b_to_a_count__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::FirstSeen => {
                            if first_seen__.is_some() {
                                return Err(serde::de::Error::duplicate_field("firstSeen"));
                            }
                            first_seen__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::LastSeen => {
                            if last_seen__.is_some() {
                                return Err(serde::de::Error::duplicate_field("lastSeen"));
                            }
                            last_seen__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                    }
                }
                Ok(GraphIpAffinity {
                    tenant_id: tenant_id__.unwrap_or_default(),
                    date: date__.unwrap_or_default(),
                    ip_a: ip_a__.unwrap_or_default(),
                    ip_b: ip_b__.unwrap_or_default(),
                    session_count: session_count__.unwrap_or_default(),
                    total_bytes: total_bytes__.unwrap_or_default(),
                    avg_duration_ms: avg_duration_ms__.unwrap_or_default(),
                    a_to_b_count: a_to_b_count__.unwrap_or_default(),
                    b_to_a_count: b_to_a_count__.unwrap_or_default(),
                    first_seen: first_seen__.unwrap_or_default(),
                    last_seen: last_seen__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.GraphIPAffinity", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for GraphQueryLog {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.tenant_id.is_empty() {
            len += 1;
        }
        if !self.query_id.is_empty() {
            len += 1;
        }
        if !self.user_id.is_empty() {
            len += 1;
        }
        if !self.query_type.is_empty() {
            len += 1;
        }
        if !self.center_ip.is_empty() {
            len += 1;
        }
        if !self.center_ips.is_empty() {
            len += 1;
        }
        if self.depth != 0 {
            len += 1;
        }
        if !self.run_id.is_empty() {
            len += 1;
        }
        if self.query_start_time != 0 {
            len += 1;
        }
        if self.query_end_time != 0 {
            len += 1;
        }
        if self.node_count != 0 {
            len += 1;
        }
        if self.edge_count != 0 {
            len += 1;
        }
        if self.path_count != 0 {
            len += 1;
        }
        if self.result_size_bytes != 0 {
            len += 1;
        }
        if self.duration_ms != 0 {
            len += 1;
        }
        if self.cache_hit != 0 {
            len += 1;
        }
        if self.ch_query_count != 0 {
            len += 1;
        }
        if self.ch_total_duration_ms != 0 {
            len += 1;
        }
        if self.ch_rows_read != 0 {
            len += 1;
        }
        if self.ch_bytes_read != 0 {
            len += 1;
        }
        if !self.status.is_empty() {
            len += 1;
        }
        if !self.error_code.is_empty() {
            len += 1;
        }
        if !self.error_message.is_empty() {
            len += 1;
        }
        if !self.trace_id.is_empty() {
            len += 1;
        }
        if !self.client_ip.is_empty() {
            len += 1;
        }
        if !self.user_agent.is_empty() {
            len += 1;
        }
        if self.created_at != 0 {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.GraphQueryLog", len)?;
        if !self.tenant_id.is_empty() {
            struct_ser.serialize_field("tenantId", &self.tenant_id)?;
        }
        if !self.query_id.is_empty() {
            struct_ser.serialize_field("queryId", &self.query_id)?;
        }
        if !self.user_id.is_empty() {
            struct_ser.serialize_field("userId", &self.user_id)?;
        }
        if !self.query_type.is_empty() {
            struct_ser.serialize_field("queryType", &self.query_type)?;
        }
        if !self.center_ip.is_empty() {
            struct_ser.serialize_field("centerIp", &self.center_ip)?;
        }
        if !self.center_ips.is_empty() {
            struct_ser.serialize_field("centerIps", &self.center_ips)?;
        }
        if self.depth != 0 {
            struct_ser.serialize_field("depth", &self.depth)?;
        }
        if !self.run_id.is_empty() {
            struct_ser.serialize_field("runId", &self.run_id)?;
        }
        if self.query_start_time != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("queryStartTime", ToString::to_string(&self.query_start_time).as_str())?;
        }
        if self.query_end_time != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("queryEndTime", ToString::to_string(&self.query_end_time).as_str())?;
        }
        if self.node_count != 0 {
            struct_ser.serialize_field("nodeCount", &self.node_count)?;
        }
        if self.edge_count != 0 {
            struct_ser.serialize_field("edgeCount", &self.edge_count)?;
        }
        if self.path_count != 0 {
            struct_ser.serialize_field("pathCount", &self.path_count)?;
        }
        if self.result_size_bytes != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("resultSizeBytes", ToString::to_string(&self.result_size_bytes).as_str())?;
        }
        if self.duration_ms != 0 {
            struct_ser.serialize_field("durationMs", &self.duration_ms)?;
        }
        if self.cache_hit != 0 {
            struct_ser.serialize_field("cacheHit", &self.cache_hit)?;
        }
        if self.ch_query_count != 0 {
            struct_ser.serialize_field("chQueryCount", &self.ch_query_count)?;
        }
        if self.ch_total_duration_ms != 0 {
            struct_ser.serialize_field("chTotalDurationMs", &self.ch_total_duration_ms)?;
        }
        if self.ch_rows_read != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("chRowsRead", ToString::to_string(&self.ch_rows_read).as_str())?;
        }
        if self.ch_bytes_read != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("chBytesRead", ToString::to_string(&self.ch_bytes_read).as_str())?;
        }
        if !self.status.is_empty() {
            struct_ser.serialize_field("status", &self.status)?;
        }
        if !self.error_code.is_empty() {
            struct_ser.serialize_field("errorCode", &self.error_code)?;
        }
        if !self.error_message.is_empty() {
            struct_ser.serialize_field("errorMessage", &self.error_message)?;
        }
        if !self.trace_id.is_empty() {
            struct_ser.serialize_field("traceId", &self.trace_id)?;
        }
        if !self.client_ip.is_empty() {
            struct_ser.serialize_field("clientIp", &self.client_ip)?;
        }
        if !self.user_agent.is_empty() {
            struct_ser.serialize_field("userAgent", &self.user_agent)?;
        }
        if self.created_at != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("createdAt", ToString::to_string(&self.created_at).as_str())?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for GraphQueryLog {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "tenant_id",
            "tenantId",
            "query_id",
            "queryId",
            "user_id",
            "userId",
            "query_type",
            "queryType",
            "center_ip",
            "centerIp",
            "center_ips",
            "centerIps",
            "depth",
            "run_id",
            "runId",
            "query_start_time",
            "queryStartTime",
            "query_end_time",
            "queryEndTime",
            "node_count",
            "nodeCount",
            "edge_count",
            "edgeCount",
            "path_count",
            "pathCount",
            "result_size_bytes",
            "resultSizeBytes",
            "duration_ms",
            "durationMs",
            "cache_hit",
            "cacheHit",
            "ch_query_count",
            "chQueryCount",
            "ch_total_duration_ms",
            "chTotalDurationMs",
            "ch_rows_read",
            "chRowsRead",
            "ch_bytes_read",
            "chBytesRead",
            "status",
            "error_code",
            "errorCode",
            "error_message",
            "errorMessage",
            "trace_id",
            "traceId",
            "client_ip",
            "clientIp",
            "user_agent",
            "userAgent",
            "created_at",
            "createdAt",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            TenantId,
            QueryId,
            UserId,
            QueryType,
            CenterIp,
            CenterIps,
            Depth,
            RunId,
            QueryStartTime,
            QueryEndTime,
            NodeCount,
            EdgeCount,
            PathCount,
            ResultSizeBytes,
            DurationMs,
            CacheHit,
            ChQueryCount,
            ChTotalDurationMs,
            ChRowsRead,
            ChBytesRead,
            Status,
            ErrorCode,
            ErrorMessage,
            TraceId,
            ClientIp,
            UserAgent,
            CreatedAt,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "tenantId" | "tenant_id" => Ok(GeneratedField::TenantId),
                            "queryId" | "query_id" => Ok(GeneratedField::QueryId),
                            "userId" | "user_id" => Ok(GeneratedField::UserId),
                            "queryType" | "query_type" => Ok(GeneratedField::QueryType),
                            "centerIp" | "center_ip" => Ok(GeneratedField::CenterIp),
                            "centerIps" | "center_ips" => Ok(GeneratedField::CenterIps),
                            "depth" => Ok(GeneratedField::Depth),
                            "runId" | "run_id" => Ok(GeneratedField::RunId),
                            "queryStartTime" | "query_start_time" => Ok(GeneratedField::QueryStartTime),
                            "queryEndTime" | "query_end_time" => Ok(GeneratedField::QueryEndTime),
                            "nodeCount" | "node_count" => Ok(GeneratedField::NodeCount),
                            "edgeCount" | "edge_count" => Ok(GeneratedField::EdgeCount),
                            "pathCount" | "path_count" => Ok(GeneratedField::PathCount),
                            "resultSizeBytes" | "result_size_bytes" => Ok(GeneratedField::ResultSizeBytes),
                            "durationMs" | "duration_ms" => Ok(GeneratedField::DurationMs),
                            "cacheHit" | "cache_hit" => Ok(GeneratedField::CacheHit),
                            "chQueryCount" | "ch_query_count" => Ok(GeneratedField::ChQueryCount),
                            "chTotalDurationMs" | "ch_total_duration_ms" => Ok(GeneratedField::ChTotalDurationMs),
                            "chRowsRead" | "ch_rows_read" => Ok(GeneratedField::ChRowsRead),
                            "chBytesRead" | "ch_bytes_read" => Ok(GeneratedField::ChBytesRead),
                            "status" => Ok(GeneratedField::Status),
                            "errorCode" | "error_code" => Ok(GeneratedField::ErrorCode),
                            "errorMessage" | "error_message" => Ok(GeneratedField::ErrorMessage),
                            "traceId" | "trace_id" => Ok(GeneratedField::TraceId),
                            "clientIp" | "client_ip" => Ok(GeneratedField::ClientIp),
                            "userAgent" | "user_agent" => Ok(GeneratedField::UserAgent),
                            "createdAt" | "created_at" => Ok(GeneratedField::CreatedAt),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = GraphQueryLog;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.GraphQueryLog")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<GraphQueryLog, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut tenant_id__ = None;
                let mut query_id__ = None;
                let mut user_id__ = None;
                let mut query_type__ = None;
                let mut center_ip__ = None;
                let mut center_ips__ = None;
                let mut depth__ = None;
                let mut run_id__ = None;
                let mut query_start_time__ = None;
                let mut query_end_time__ = None;
                let mut node_count__ = None;
                let mut edge_count__ = None;
                let mut path_count__ = None;
                let mut result_size_bytes__ = None;
                let mut duration_ms__ = None;
                let mut cache_hit__ = None;
                let mut ch_query_count__ = None;
                let mut ch_total_duration_ms__ = None;
                let mut ch_rows_read__ = None;
                let mut ch_bytes_read__ = None;
                let mut status__ = None;
                let mut error_code__ = None;
                let mut error_message__ = None;
                let mut trace_id__ = None;
                let mut client_ip__ = None;
                let mut user_agent__ = None;
                let mut created_at__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::TenantId => {
                            if tenant_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("tenantId"));
                            }
                            tenant_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::QueryId => {
                            if query_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("queryId"));
                            }
                            query_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::UserId => {
                            if user_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("userId"));
                            }
                            user_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::QueryType => {
                            if query_type__.is_some() {
                                return Err(serde::de::Error::duplicate_field("queryType"));
                            }
                            query_type__ = Some(map_.next_value()?);
                        }
                        GeneratedField::CenterIp => {
                            if center_ip__.is_some() {
                                return Err(serde::de::Error::duplicate_field("centerIp"));
                            }
                            center_ip__ = Some(map_.next_value()?);
                        }
                        GeneratedField::CenterIps => {
                            if center_ips__.is_some() {
                                return Err(serde::de::Error::duplicate_field("centerIps"));
                            }
                            center_ips__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Depth => {
                            if depth__.is_some() {
                                return Err(serde::de::Error::duplicate_field("depth"));
                            }
                            depth__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::RunId => {
                            if run_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("runId"));
                            }
                            run_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::QueryStartTime => {
                            if query_start_time__.is_some() {
                                return Err(serde::de::Error::duplicate_field("queryStartTime"));
                            }
                            query_start_time__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::QueryEndTime => {
                            if query_end_time__.is_some() {
                                return Err(serde::de::Error::duplicate_field("queryEndTime"));
                            }
                            query_end_time__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::NodeCount => {
                            if node_count__.is_some() {
                                return Err(serde::de::Error::duplicate_field("nodeCount"));
                            }
                            node_count__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::EdgeCount => {
                            if edge_count__.is_some() {
                                return Err(serde::de::Error::duplicate_field("edgeCount"));
                            }
                            edge_count__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::PathCount => {
                            if path_count__.is_some() {
                                return Err(serde::de::Error::duplicate_field("pathCount"));
                            }
                            path_count__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::ResultSizeBytes => {
                            if result_size_bytes__.is_some() {
                                return Err(serde::de::Error::duplicate_field("resultSizeBytes"));
                            }
                            result_size_bytes__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::DurationMs => {
                            if duration_ms__.is_some() {
                                return Err(serde::de::Error::duplicate_field("durationMs"));
                            }
                            duration_ms__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::CacheHit => {
                            if cache_hit__.is_some() {
                                return Err(serde::de::Error::duplicate_field("cacheHit"));
                            }
                            cache_hit__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::ChQueryCount => {
                            if ch_query_count__.is_some() {
                                return Err(serde::de::Error::duplicate_field("chQueryCount"));
                            }
                            ch_query_count__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::ChTotalDurationMs => {
                            if ch_total_duration_ms__.is_some() {
                                return Err(serde::de::Error::duplicate_field("chTotalDurationMs"));
                            }
                            ch_total_duration_ms__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::ChRowsRead => {
                            if ch_rows_read__.is_some() {
                                return Err(serde::de::Error::duplicate_field("chRowsRead"));
                            }
                            ch_rows_read__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::ChBytesRead => {
                            if ch_bytes_read__.is_some() {
                                return Err(serde::de::Error::duplicate_field("chBytesRead"));
                            }
                            ch_bytes_read__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::Status => {
                            if status__.is_some() {
                                return Err(serde::de::Error::duplicate_field("status"));
                            }
                            status__ = Some(map_.next_value()?);
                        }
                        GeneratedField::ErrorCode => {
                            if error_code__.is_some() {
                                return Err(serde::de::Error::duplicate_field("errorCode"));
                            }
                            error_code__ = Some(map_.next_value()?);
                        }
                        GeneratedField::ErrorMessage => {
                            if error_message__.is_some() {
                                return Err(serde::de::Error::duplicate_field("errorMessage"));
                            }
                            error_message__ = Some(map_.next_value()?);
                        }
                        GeneratedField::TraceId => {
                            if trace_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("traceId"));
                            }
                            trace_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::ClientIp => {
                            if client_ip__.is_some() {
                                return Err(serde::de::Error::duplicate_field("clientIp"));
                            }
                            client_ip__ = Some(map_.next_value()?);
                        }
                        GeneratedField::UserAgent => {
                            if user_agent__.is_some() {
                                return Err(serde::de::Error::duplicate_field("userAgent"));
                            }
                            user_agent__ = Some(map_.next_value()?);
                        }
                        GeneratedField::CreatedAt => {
                            if created_at__.is_some() {
                                return Err(serde::de::Error::duplicate_field("createdAt"));
                            }
                            created_at__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                    }
                }
                Ok(GraphQueryLog {
                    tenant_id: tenant_id__.unwrap_or_default(),
                    query_id: query_id__.unwrap_or_default(),
                    user_id: user_id__.unwrap_or_default(),
                    query_type: query_type__.unwrap_or_default(),
                    center_ip: center_ip__.unwrap_or_default(),
                    center_ips: center_ips__.unwrap_or_default(),
                    depth: depth__.unwrap_or_default(),
                    run_id: run_id__.unwrap_or_default(),
                    query_start_time: query_start_time__.unwrap_or_default(),
                    query_end_time: query_end_time__.unwrap_or_default(),
                    node_count: node_count__.unwrap_or_default(),
                    edge_count: edge_count__.unwrap_or_default(),
                    path_count: path_count__.unwrap_or_default(),
                    result_size_bytes: result_size_bytes__.unwrap_or_default(),
                    duration_ms: duration_ms__.unwrap_or_default(),
                    cache_hit: cache_hit__.unwrap_or_default(),
                    ch_query_count: ch_query_count__.unwrap_or_default(),
                    ch_total_duration_ms: ch_total_duration_ms__.unwrap_or_default(),
                    ch_rows_read: ch_rows_read__.unwrap_or_default(),
                    ch_bytes_read: ch_bytes_read__.unwrap_or_default(),
                    status: status__.unwrap_or_default(),
                    error_code: error_code__.unwrap_or_default(),
                    error_message: error_message__.unwrap_or_default(),
                    trace_id: trace_id__.unwrap_or_default(),
                    client_ip: client_ip__.unwrap_or_default(),
                    user_agent: user_agent__.unwrap_or_default(),
                    created_at: created_at__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.GraphQueryLog", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for GraphQueryLogBatch {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.logs.is_empty() {
            len += 1;
        }
        if !self.batch_id.is_empty() {
            len += 1;
        }
        if !self.tenant_id.is_empty() {
            len += 1;
        }
        if self.created_at != 0 {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.GraphQueryLogBatch", len)?;
        if !self.logs.is_empty() {
            struct_ser.serialize_field("logs", &self.logs)?;
        }
        if !self.batch_id.is_empty() {
            struct_ser.serialize_field("batchId", &self.batch_id)?;
        }
        if !self.tenant_id.is_empty() {
            struct_ser.serialize_field("tenantId", &self.tenant_id)?;
        }
        if self.created_at != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("createdAt", ToString::to_string(&self.created_at).as_str())?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for GraphQueryLogBatch {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "logs",
            "batch_id",
            "batchId",
            "tenant_id",
            "tenantId",
            "created_at",
            "createdAt",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Logs,
            BatchId,
            TenantId,
            CreatedAt,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "logs" => Ok(GeneratedField::Logs),
                            "batchId" | "batch_id" => Ok(GeneratedField::BatchId),
                            "tenantId" | "tenant_id" => Ok(GeneratedField::TenantId),
                            "createdAt" | "created_at" => Ok(GeneratedField::CreatedAt),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = GraphQueryLogBatch;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.GraphQueryLogBatch")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<GraphQueryLogBatch, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut logs__ = None;
                let mut batch_id__ = None;
                let mut tenant_id__ = None;
                let mut created_at__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Logs => {
                            if logs__.is_some() {
                                return Err(serde::de::Error::duplicate_field("logs"));
                            }
                            logs__ = Some(map_.next_value()?);
                        }
                        GeneratedField::BatchId => {
                            if batch_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("batchId"));
                            }
                            batch_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::TenantId => {
                            if tenant_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("tenantId"));
                            }
                            tenant_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::CreatedAt => {
                            if created_at__.is_some() {
                                return Err(serde::de::Error::duplicate_field("createdAt"));
                            }
                            created_at__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                    }
                }
                Ok(GraphQueryLogBatch {
                    logs: logs__.unwrap_or_default(),
                    batch_id: batch_id__.unwrap_or_default(),
                    tenant_id: tenant_id__.unwrap_or_default(),
                    created_at: created_at__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.GraphQueryLogBatch", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for GraphSlowQuery {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.tenant_id.is_empty() {
            len += 1;
        }
        if !self.query_id.is_empty() {
            len += 1;
        }
        if !self.query_type.is_empty() {
            len += 1;
        }
        if !self.center_ip.is_empty() {
            len += 1;
        }
        if self.depth != 0 {
            len += 1;
        }
        if !self.run_id.is_empty() {
            len += 1;
        }
        if self.duration_ms != 0 {
            len += 1;
        }
        if self.node_count != 0 {
            len += 1;
        }
        if self.edge_count != 0 {
            len += 1;
        }
        if self.ch_rows_read != 0 {
            len += 1;
        }
        if self.ch_bytes_read != 0 {
            len += 1;
        }
        if !self.error_message.is_empty() {
            len += 1;
        }
        if self.created_at != 0 {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.GraphSlowQuery", len)?;
        if !self.tenant_id.is_empty() {
            struct_ser.serialize_field("tenantId", &self.tenant_id)?;
        }
        if !self.query_id.is_empty() {
            struct_ser.serialize_field("queryId", &self.query_id)?;
        }
        if !self.query_type.is_empty() {
            struct_ser.serialize_field("queryType", &self.query_type)?;
        }
        if !self.center_ip.is_empty() {
            struct_ser.serialize_field("centerIp", &self.center_ip)?;
        }
        if self.depth != 0 {
            struct_ser.serialize_field("depth", &self.depth)?;
        }
        if !self.run_id.is_empty() {
            struct_ser.serialize_field("runId", &self.run_id)?;
        }
        if self.duration_ms != 0 {
            struct_ser.serialize_field("durationMs", &self.duration_ms)?;
        }
        if self.node_count != 0 {
            struct_ser.serialize_field("nodeCount", &self.node_count)?;
        }
        if self.edge_count != 0 {
            struct_ser.serialize_field("edgeCount", &self.edge_count)?;
        }
        if self.ch_rows_read != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("chRowsRead", ToString::to_string(&self.ch_rows_read).as_str())?;
        }
        if self.ch_bytes_read != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("chBytesRead", ToString::to_string(&self.ch_bytes_read).as_str())?;
        }
        if !self.error_message.is_empty() {
            struct_ser.serialize_field("errorMessage", &self.error_message)?;
        }
        if self.created_at != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("createdAt", ToString::to_string(&self.created_at).as_str())?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for GraphSlowQuery {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "tenant_id",
            "tenantId",
            "query_id",
            "queryId",
            "query_type",
            "queryType",
            "center_ip",
            "centerIp",
            "depth",
            "run_id",
            "runId",
            "duration_ms",
            "durationMs",
            "node_count",
            "nodeCount",
            "edge_count",
            "edgeCount",
            "ch_rows_read",
            "chRowsRead",
            "ch_bytes_read",
            "chBytesRead",
            "error_message",
            "errorMessage",
            "created_at",
            "createdAt",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            TenantId,
            QueryId,
            QueryType,
            CenterIp,
            Depth,
            RunId,
            DurationMs,
            NodeCount,
            EdgeCount,
            ChRowsRead,
            ChBytesRead,
            ErrorMessage,
            CreatedAt,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "tenantId" | "tenant_id" => Ok(GeneratedField::TenantId),
                            "queryId" | "query_id" => Ok(GeneratedField::QueryId),
                            "queryType" | "query_type" => Ok(GeneratedField::QueryType),
                            "centerIp" | "center_ip" => Ok(GeneratedField::CenterIp),
                            "depth" => Ok(GeneratedField::Depth),
                            "runId" | "run_id" => Ok(GeneratedField::RunId),
                            "durationMs" | "duration_ms" => Ok(GeneratedField::DurationMs),
                            "nodeCount" | "node_count" => Ok(GeneratedField::NodeCount),
                            "edgeCount" | "edge_count" => Ok(GeneratedField::EdgeCount),
                            "chRowsRead" | "ch_rows_read" => Ok(GeneratedField::ChRowsRead),
                            "chBytesRead" | "ch_bytes_read" => Ok(GeneratedField::ChBytesRead),
                            "errorMessage" | "error_message" => Ok(GeneratedField::ErrorMessage),
                            "createdAt" | "created_at" => Ok(GeneratedField::CreatedAt),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = GraphSlowQuery;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.GraphSlowQuery")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<GraphSlowQuery, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut tenant_id__ = None;
                let mut query_id__ = None;
                let mut query_type__ = None;
                let mut center_ip__ = None;
                let mut depth__ = None;
                let mut run_id__ = None;
                let mut duration_ms__ = None;
                let mut node_count__ = None;
                let mut edge_count__ = None;
                let mut ch_rows_read__ = None;
                let mut ch_bytes_read__ = None;
                let mut error_message__ = None;
                let mut created_at__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::TenantId => {
                            if tenant_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("tenantId"));
                            }
                            tenant_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::QueryId => {
                            if query_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("queryId"));
                            }
                            query_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::QueryType => {
                            if query_type__.is_some() {
                                return Err(serde::de::Error::duplicate_field("queryType"));
                            }
                            query_type__ = Some(map_.next_value()?);
                        }
                        GeneratedField::CenterIp => {
                            if center_ip__.is_some() {
                                return Err(serde::de::Error::duplicate_field("centerIp"));
                            }
                            center_ip__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Depth => {
                            if depth__.is_some() {
                                return Err(serde::de::Error::duplicate_field("depth"));
                            }
                            depth__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::RunId => {
                            if run_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("runId"));
                            }
                            run_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::DurationMs => {
                            if duration_ms__.is_some() {
                                return Err(serde::de::Error::duplicate_field("durationMs"));
                            }
                            duration_ms__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::NodeCount => {
                            if node_count__.is_some() {
                                return Err(serde::de::Error::duplicate_field("nodeCount"));
                            }
                            node_count__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::EdgeCount => {
                            if edge_count__.is_some() {
                                return Err(serde::de::Error::duplicate_field("edgeCount"));
                            }
                            edge_count__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::ChRowsRead => {
                            if ch_rows_read__.is_some() {
                                return Err(serde::de::Error::duplicate_field("chRowsRead"));
                            }
                            ch_rows_read__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::ChBytesRead => {
                            if ch_bytes_read__.is_some() {
                                return Err(serde::de::Error::duplicate_field("chBytesRead"));
                            }
                            ch_bytes_read__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::ErrorMessage => {
                            if error_message__.is_some() {
                                return Err(serde::de::Error::duplicate_field("errorMessage"));
                            }
                            error_message__ = Some(map_.next_value()?);
                        }
                        GeneratedField::CreatedAt => {
                            if created_at__.is_some() {
                                return Err(serde::de::Error::duplicate_field("createdAt"));
                            }
                            created_at__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                    }
                }
                Ok(GraphSlowQuery {
                    tenant_id: tenant_id__.unwrap_or_default(),
                    query_id: query_id__.unwrap_or_default(),
                    query_type: query_type__.unwrap_or_default(),
                    center_ip: center_ip__.unwrap_or_default(),
                    depth: depth__.unwrap_or_default(),
                    run_id: run_id__.unwrap_or_default(),
                    duration_ms: duration_ms__.unwrap_or_default(),
                    node_count: node_count__.unwrap_or_default(),
                    edge_count: edge_count__.unwrap_or_default(),
                    ch_rows_read: ch_rows_read__.unwrap_or_default(),
                    ch_bytes_read: ch_bytes_read__.unwrap_or_default(),
                    error_message: error_message__.unwrap_or_default(),
                    created_at: created_at__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.GraphSlowQuery", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for GraphStatsBatch {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.cache_stats.is_empty() {
            len += 1;
        }
        if !self.hot_ips.is_empty() {
            len += 1;
        }
        if !self.slow_queries.is_empty() {
            len += 1;
        }
        if !self.ip_affinities.is_empty() {
            len += 1;
        }
        if !self.batch_id.is_empty() {
            len += 1;
        }
        if !self.tenant_id.is_empty() {
            len += 1;
        }
        if self.created_at != 0 {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.GraphStatsBatch", len)?;
        if !self.cache_stats.is_empty() {
            struct_ser.serialize_field("cacheStats", &self.cache_stats)?;
        }
        if !self.hot_ips.is_empty() {
            struct_ser.serialize_field("hotIps", &self.hot_ips)?;
        }
        if !self.slow_queries.is_empty() {
            struct_ser.serialize_field("slowQueries", &self.slow_queries)?;
        }
        if !self.ip_affinities.is_empty() {
            struct_ser.serialize_field("ipAffinities", &self.ip_affinities)?;
        }
        if !self.batch_id.is_empty() {
            struct_ser.serialize_field("batchId", &self.batch_id)?;
        }
        if !self.tenant_id.is_empty() {
            struct_ser.serialize_field("tenantId", &self.tenant_id)?;
        }
        if self.created_at != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("createdAt", ToString::to_string(&self.created_at).as_str())?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for GraphStatsBatch {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "cache_stats",
            "cacheStats",
            "hot_ips",
            "hotIps",
            "slow_queries",
            "slowQueries",
            "ip_affinities",
            "ipAffinities",
            "batch_id",
            "batchId",
            "tenant_id",
            "tenantId",
            "created_at",
            "createdAt",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            CacheStats,
            HotIps,
            SlowQueries,
            IpAffinities,
            BatchId,
            TenantId,
            CreatedAt,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "cacheStats" | "cache_stats" => Ok(GeneratedField::CacheStats),
                            "hotIps" | "hot_ips" => Ok(GeneratedField::HotIps),
                            "slowQueries" | "slow_queries" => Ok(GeneratedField::SlowQueries),
                            "ipAffinities" | "ip_affinities" => Ok(GeneratedField::IpAffinities),
                            "batchId" | "batch_id" => Ok(GeneratedField::BatchId),
                            "tenantId" | "tenant_id" => Ok(GeneratedField::TenantId),
                            "createdAt" | "created_at" => Ok(GeneratedField::CreatedAt),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = GraphStatsBatch;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.GraphStatsBatch")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<GraphStatsBatch, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut cache_stats__ = None;
                let mut hot_ips__ = None;
                let mut slow_queries__ = None;
                let mut ip_affinities__ = None;
                let mut batch_id__ = None;
                let mut tenant_id__ = None;
                let mut created_at__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::CacheStats => {
                            if cache_stats__.is_some() {
                                return Err(serde::de::Error::duplicate_field("cacheStats"));
                            }
                            cache_stats__ = Some(map_.next_value()?);
                        }
                        GeneratedField::HotIps => {
                            if hot_ips__.is_some() {
                                return Err(serde::de::Error::duplicate_field("hotIps"));
                            }
                            hot_ips__ = Some(map_.next_value()?);
                        }
                        GeneratedField::SlowQueries => {
                            if slow_queries__.is_some() {
                                return Err(serde::de::Error::duplicate_field("slowQueries"));
                            }
                            slow_queries__ = Some(map_.next_value()?);
                        }
                        GeneratedField::IpAffinities => {
                            if ip_affinities__.is_some() {
                                return Err(serde::de::Error::duplicate_field("ipAffinities"));
                            }
                            ip_affinities__ = Some(map_.next_value()?);
                        }
                        GeneratedField::BatchId => {
                            if batch_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("batchId"));
                            }
                            batch_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::TenantId => {
                            if tenant_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("tenantId"));
                            }
                            tenant_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::CreatedAt => {
                            if created_at__.is_some() {
                                return Err(serde::de::Error::duplicate_field("createdAt"));
                            }
                            created_at__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                    }
                }
                Ok(GraphStatsBatch {
                    cache_stats: cache_stats__.unwrap_or_default(),
                    hot_ips: hot_ips__.unwrap_or_default(),
                    slow_queries: slow_queries__.unwrap_or_default(),
                    ip_affinities: ip_affinities__.unwrap_or_default(),
                    batch_id: batch_id__.unwrap_or_default(),
                    tenant_id: tenant_id__.unwrap_or_default(),
                    created_at: created_at__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.GraphStatsBatch", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for HardwareInfo {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.cpu_model.is_empty() {
            len += 1;
        }
        if self.cpu_cores != 0 {
            len += 1;
        }
        if self.memory_mb != 0 {
            len += 1;
        }
        if !self.os_version.is_empty() {
            len += 1;
        }
        if !self.nics.is_empty() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.HardwareInfo", len)?;
        if !self.cpu_model.is_empty() {
            struct_ser.serialize_field("cpuModel", &self.cpu_model)?;
        }
        if self.cpu_cores != 0 {
            struct_ser.serialize_field("cpuCores", &self.cpu_cores)?;
        }
        if self.memory_mb != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("memoryMb", ToString::to_string(&self.memory_mb).as_str())?;
        }
        if !self.os_version.is_empty() {
            struct_ser.serialize_field("osVersion", &self.os_version)?;
        }
        if !self.nics.is_empty() {
            struct_ser.serialize_field("nics", &self.nics)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for HardwareInfo {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "cpu_model",
            "cpuModel",
            "cpu_cores",
            "cpuCores",
            "memory_mb",
            "memoryMb",
            "os_version",
            "osVersion",
            "nics",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            CpuModel,
            CpuCores,
            MemoryMb,
            OsVersion,
            Nics,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "cpuModel" | "cpu_model" => Ok(GeneratedField::CpuModel),
                            "cpuCores" | "cpu_cores" => Ok(GeneratedField::CpuCores),
                            "memoryMb" | "memory_mb" => Ok(GeneratedField::MemoryMb),
                            "osVersion" | "os_version" => Ok(GeneratedField::OsVersion),
                            "nics" => Ok(GeneratedField::Nics),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = HardwareInfo;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.HardwareInfo")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<HardwareInfo, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut cpu_model__ = None;
                let mut cpu_cores__ = None;
                let mut memory_mb__ = None;
                let mut os_version__ = None;
                let mut nics__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::CpuModel => {
                            if cpu_model__.is_some() {
                                return Err(serde::de::Error::duplicate_field("cpuModel"));
                            }
                            cpu_model__ = Some(map_.next_value()?);
                        }
                        GeneratedField::CpuCores => {
                            if cpu_cores__.is_some() {
                                return Err(serde::de::Error::duplicate_field("cpuCores"));
                            }
                            cpu_cores__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::MemoryMb => {
                            if memory_mb__.is_some() {
                                return Err(serde::de::Error::duplicate_field("memoryMb"));
                            }
                            memory_mb__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::OsVersion => {
                            if os_version__.is_some() {
                                return Err(serde::de::Error::duplicate_field("osVersion"));
                            }
                            os_version__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Nics => {
                            if nics__.is_some() {
                                return Err(serde::de::Error::duplicate_field("nics"));
                            }
                            nics__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(HardwareInfo {
                    cpu_model: cpu_model__.unwrap_or_default(),
                    cpu_cores: cpu_cores__.unwrap_or_default(),
                    memory_mb: memory_mb__.unwrap_or_default(),
                    os_version: os_version__.unwrap_or_default(),
                    nics: nics__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.HardwareInfo", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for HeartbeatRequest {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.tenant_id.is_empty() {
            len += 1;
        }
        if !self.probe_id.is_empty() {
            len += 1;
        }
        if self.status.is_some() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.HeartbeatRequest", len)?;
        if !self.tenant_id.is_empty() {
            struct_ser.serialize_field("tenantId", &self.tenant_id)?;
        }
        if !self.probe_id.is_empty() {
            struct_ser.serialize_field("probeId", &self.probe_id)?;
        }
        if let Some(v) = self.status.as_ref() {
            struct_ser.serialize_field("status", v)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for HeartbeatRequest {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "tenant_id",
            "tenantId",
            "probe_id",
            "probeId",
            "status",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            TenantId,
            ProbeId,
            Status,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "tenantId" | "tenant_id" => Ok(GeneratedField::TenantId),
                            "probeId" | "probe_id" => Ok(GeneratedField::ProbeId),
                            "status" => Ok(GeneratedField::Status),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = HeartbeatRequest;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.HeartbeatRequest")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<HeartbeatRequest, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut tenant_id__ = None;
                let mut probe_id__ = None;
                let mut status__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::TenantId => {
                            if tenant_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("tenantId"));
                            }
                            tenant_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::ProbeId => {
                            if probe_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("probeId"));
                            }
                            probe_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Status => {
                            if status__.is_some() {
                                return Err(serde::de::Error::duplicate_field("status"));
                            }
                            status__ = map_.next_value()?;
                        }
                    }
                }
                Ok(HeartbeatRequest {
                    tenant_id: tenant_id__.unwrap_or_default(),
                    probe_id: probe_id__.unwrap_or_default(),
                    status: status__,
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.HeartbeatRequest", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for HeartbeatResponse {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if self.ok {
            len += 1;
        }
        if self.config.is_some() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.HeartbeatResponse", len)?;
        if self.ok {
            struct_ser.serialize_field("ok", &self.ok)?;
        }
        if let Some(v) = self.config.as_ref() {
            struct_ser.serialize_field("config", v)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for HeartbeatResponse {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "ok",
            "config",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Ok,
            Config,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "ok" => Ok(GeneratedField::Ok),
                            "config" => Ok(GeneratedField::Config),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = HeartbeatResponse;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.HeartbeatResponse")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<HeartbeatResponse, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut ok__ = None;
                let mut config__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Ok => {
                            if ok__.is_some() {
                                return Err(serde::de::Error::duplicate_field("ok"));
                            }
                            ok__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Config => {
                            if config__.is_some() {
                                return Err(serde::de::Error::duplicate_field("config"));
                            }
                            config__ = map_.next_value()?;
                        }
                    }
                }
                Ok(HeartbeatResponse {
                    ok: ok__.unwrap_or_default(),
                    config: config__,
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.HeartbeatResponse", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for InterArrivalStats {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if self.min_ms != 0. {
            len += 1;
        }
        if self.max_ms != 0. {
            len += 1;
        }
        if self.mean_ms != 0. {
            len += 1;
        }
        if self.std_ms != 0. {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.InterArrivalStats", len)?;
        if self.min_ms != 0. {
            struct_ser.serialize_field("minMs", &self.min_ms)?;
        }
        if self.max_ms != 0. {
            struct_ser.serialize_field("maxMs", &self.max_ms)?;
        }
        if self.mean_ms != 0. {
            struct_ser.serialize_field("meanMs", &self.mean_ms)?;
        }
        if self.std_ms != 0. {
            struct_ser.serialize_field("stdMs", &self.std_ms)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for InterArrivalStats {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "min_ms",
            "minMs",
            "max_ms",
            "maxMs",
            "mean_ms",
            "meanMs",
            "std_ms",
            "stdMs",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            MinMs,
            MaxMs,
            MeanMs,
            StdMs,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "minMs" | "min_ms" => Ok(GeneratedField::MinMs),
                            "maxMs" | "max_ms" => Ok(GeneratedField::MaxMs),
                            "meanMs" | "mean_ms" => Ok(GeneratedField::MeanMs),
                            "stdMs" | "std_ms" => Ok(GeneratedField::StdMs),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = InterArrivalStats;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.InterArrivalStats")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<InterArrivalStats, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut min_ms__ = None;
                let mut max_ms__ = None;
                let mut mean_ms__ = None;
                let mut std_ms__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::MinMs => {
                            if min_ms__.is_some() {
                                return Err(serde::de::Error::duplicate_field("minMs"));
                            }
                            min_ms__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::MaxMs => {
                            if max_ms__.is_some() {
                                return Err(serde::de::Error::duplicate_field("maxMs"));
                            }
                            max_ms__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::MeanMs => {
                            if mean_ms__.is_some() {
                                return Err(serde::de::Error::duplicate_field("meanMs"));
                            }
                            mean_ms__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::StdMs => {
                            if std_ms__.is_some() {
                                return Err(serde::de::Error::duplicate_field("stdMs"));
                            }
                            std_ms__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                    }
                }
                Ok(InterArrivalStats {
                    min_ms: min_ms__.unwrap_or_default(),
                    max_ms: max_ms__.unwrap_or_default(),
                    mean_ms: mean_ms__.unwrap_or_default(),
                    std_ms: std_ms__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.InterArrivalStats", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for InterfaceStatus {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.name.is_empty() {
            len += 1;
        }
        if self.link_up {
            len += 1;
        }
        if self.speed_mbps != 0 {
            len += 1;
        }
        if self.rx_packets != 0 {
            len += 1;
        }
        if self.tx_packets != 0 {
            len += 1;
        }
        if self.rx_bytes != 0 {
            len += 1;
        }
        if self.tx_bytes != 0 {
            len += 1;
        }
        if self.rx_errors != 0 {
            len += 1;
        }
        if self.tx_errors != 0 {
            len += 1;
        }
        if self.rx_crc_errors != 0 {
            len += 1;
        }
        if self.rx_dropped != 0 {
            len += 1;
        }
        if self.collisions != 0 {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.InterfaceStatus", len)?;
        if !self.name.is_empty() {
            struct_ser.serialize_field("name", &self.name)?;
        }
        if self.link_up {
            struct_ser.serialize_field("linkUp", &self.link_up)?;
        }
        if self.speed_mbps != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("speedMbps", ToString::to_string(&self.speed_mbps).as_str())?;
        }
        if self.rx_packets != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("rxPackets", ToString::to_string(&self.rx_packets).as_str())?;
        }
        if self.tx_packets != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("txPackets", ToString::to_string(&self.tx_packets).as_str())?;
        }
        if self.rx_bytes != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("rxBytes", ToString::to_string(&self.rx_bytes).as_str())?;
        }
        if self.tx_bytes != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("txBytes", ToString::to_string(&self.tx_bytes).as_str())?;
        }
        if self.rx_errors != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("rxErrors", ToString::to_string(&self.rx_errors).as_str())?;
        }
        if self.tx_errors != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("txErrors", ToString::to_string(&self.tx_errors).as_str())?;
        }
        if self.rx_crc_errors != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("rxCrcErrors", ToString::to_string(&self.rx_crc_errors).as_str())?;
        }
        if self.rx_dropped != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("rxDropped", ToString::to_string(&self.rx_dropped).as_str())?;
        }
        if self.collisions != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("collisions", ToString::to_string(&self.collisions).as_str())?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for InterfaceStatus {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "name",
            "link_up",
            "linkUp",
            "speed_mbps",
            "speedMbps",
            "rx_packets",
            "rxPackets",
            "tx_packets",
            "txPackets",
            "rx_bytes",
            "rxBytes",
            "tx_bytes",
            "txBytes",
            "rx_errors",
            "rxErrors",
            "tx_errors",
            "txErrors",
            "rx_crc_errors",
            "rxCrcErrors",
            "rx_dropped",
            "rxDropped",
            "collisions",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Name,
            LinkUp,
            SpeedMbps,
            RxPackets,
            TxPackets,
            RxBytes,
            TxBytes,
            RxErrors,
            TxErrors,
            RxCrcErrors,
            RxDropped,
            Collisions,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "name" => Ok(GeneratedField::Name),
                            "linkUp" | "link_up" => Ok(GeneratedField::LinkUp),
                            "speedMbps" | "speed_mbps" => Ok(GeneratedField::SpeedMbps),
                            "rxPackets" | "rx_packets" => Ok(GeneratedField::RxPackets),
                            "txPackets" | "tx_packets" => Ok(GeneratedField::TxPackets),
                            "rxBytes" | "rx_bytes" => Ok(GeneratedField::RxBytes),
                            "txBytes" | "tx_bytes" => Ok(GeneratedField::TxBytes),
                            "rxErrors" | "rx_errors" => Ok(GeneratedField::RxErrors),
                            "txErrors" | "tx_errors" => Ok(GeneratedField::TxErrors),
                            "rxCrcErrors" | "rx_crc_errors" => Ok(GeneratedField::RxCrcErrors),
                            "rxDropped" | "rx_dropped" => Ok(GeneratedField::RxDropped),
                            "collisions" => Ok(GeneratedField::Collisions),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = InterfaceStatus;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.InterfaceStatus")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<InterfaceStatus, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut name__ = None;
                let mut link_up__ = None;
                let mut speed_mbps__ = None;
                let mut rx_packets__ = None;
                let mut tx_packets__ = None;
                let mut rx_bytes__ = None;
                let mut tx_bytes__ = None;
                let mut rx_errors__ = None;
                let mut tx_errors__ = None;
                let mut rx_crc_errors__ = None;
                let mut rx_dropped__ = None;
                let mut collisions__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Name => {
                            if name__.is_some() {
                                return Err(serde::de::Error::duplicate_field("name"));
                            }
                            name__ = Some(map_.next_value()?);
                        }
                        GeneratedField::LinkUp => {
                            if link_up__.is_some() {
                                return Err(serde::de::Error::duplicate_field("linkUp"));
                            }
                            link_up__ = Some(map_.next_value()?);
                        }
                        GeneratedField::SpeedMbps => {
                            if speed_mbps__.is_some() {
                                return Err(serde::de::Error::duplicate_field("speedMbps"));
                            }
                            speed_mbps__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::RxPackets => {
                            if rx_packets__.is_some() {
                                return Err(serde::de::Error::duplicate_field("rxPackets"));
                            }
                            rx_packets__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::TxPackets => {
                            if tx_packets__.is_some() {
                                return Err(serde::de::Error::duplicate_field("txPackets"));
                            }
                            tx_packets__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::RxBytes => {
                            if rx_bytes__.is_some() {
                                return Err(serde::de::Error::duplicate_field("rxBytes"));
                            }
                            rx_bytes__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::TxBytes => {
                            if tx_bytes__.is_some() {
                                return Err(serde::de::Error::duplicate_field("txBytes"));
                            }
                            tx_bytes__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::RxErrors => {
                            if rx_errors__.is_some() {
                                return Err(serde::de::Error::duplicate_field("rxErrors"));
                            }
                            rx_errors__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::TxErrors => {
                            if tx_errors__.is_some() {
                                return Err(serde::de::Error::duplicate_field("txErrors"));
                            }
                            tx_errors__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::RxCrcErrors => {
                            if rx_crc_errors__.is_some() {
                                return Err(serde::de::Error::duplicate_field("rxCrcErrors"));
                            }
                            rx_crc_errors__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::RxDropped => {
                            if rx_dropped__.is_some() {
                                return Err(serde::de::Error::duplicate_field("rxDropped"));
                            }
                            rx_dropped__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::Collisions => {
                            if collisions__.is_some() {
                                return Err(serde::de::Error::duplicate_field("collisions"));
                            }
                            collisions__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                    }
                }
                Ok(InterfaceStatus {
                    name: name__.unwrap_or_default(),
                    link_up: link_up__.unwrap_or_default(),
                    speed_mbps: speed_mbps__.unwrap_or_default(),
                    rx_packets: rx_packets__.unwrap_or_default(),
                    tx_packets: tx_packets__.unwrap_or_default(),
                    rx_bytes: rx_bytes__.unwrap_or_default(),
                    tx_bytes: tx_bytes__.unwrap_or_default(),
                    rx_errors: rx_errors__.unwrap_or_default(),
                    tx_errors: tx_errors__.unwrap_or_default(),
                    rx_crc_errors: rx_crc_errors__.unwrap_or_default(),
                    rx_dropped: rx_dropped__.unwrap_or_default(),
                    collisions: collisions__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.InterfaceStatus", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for ListAssetsRequest {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.tenant_id.is_empty() {
            len += 1;
        }
        if self.page_size != 0 {
            len += 1;
        }
        if !self.page_token.is_empty() {
            len += 1;
        }
        if !self.ip_prefix.is_empty() {
            len += 1;
        }
        if !self.vendor_filter.is_empty() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.ListAssetsRequest", len)?;
        if !self.tenant_id.is_empty() {
            struct_ser.serialize_field("tenantId", &self.tenant_id)?;
        }
        if self.page_size != 0 {
            struct_ser.serialize_field("pageSize", &self.page_size)?;
        }
        if !self.page_token.is_empty() {
            struct_ser.serialize_field("pageToken", &self.page_token)?;
        }
        if !self.ip_prefix.is_empty() {
            struct_ser.serialize_field("ipPrefix", &self.ip_prefix)?;
        }
        if !self.vendor_filter.is_empty() {
            struct_ser.serialize_field("vendorFilter", &self.vendor_filter)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for ListAssetsRequest {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "tenant_id",
            "tenantId",
            "page_size",
            "pageSize",
            "page_token",
            "pageToken",
            "ip_prefix",
            "ipPrefix",
            "vendor_filter",
            "vendorFilter",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            TenantId,
            PageSize,
            PageToken,
            IpPrefix,
            VendorFilter,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "tenantId" | "tenant_id" => Ok(GeneratedField::TenantId),
                            "pageSize" | "page_size" => Ok(GeneratedField::PageSize),
                            "pageToken" | "page_token" => Ok(GeneratedField::PageToken),
                            "ipPrefix" | "ip_prefix" => Ok(GeneratedField::IpPrefix),
                            "vendorFilter" | "vendor_filter" => Ok(GeneratedField::VendorFilter),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = ListAssetsRequest;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.ListAssetsRequest")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<ListAssetsRequest, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut tenant_id__ = None;
                let mut page_size__ = None;
                let mut page_token__ = None;
                let mut ip_prefix__ = None;
                let mut vendor_filter__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::TenantId => {
                            if tenant_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("tenantId"));
                            }
                            tenant_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::PageSize => {
                            if page_size__.is_some() {
                                return Err(serde::de::Error::duplicate_field("pageSize"));
                            }
                            page_size__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::PageToken => {
                            if page_token__.is_some() {
                                return Err(serde::de::Error::duplicate_field("pageToken"));
                            }
                            page_token__ = Some(map_.next_value()?);
                        }
                        GeneratedField::IpPrefix => {
                            if ip_prefix__.is_some() {
                                return Err(serde::de::Error::duplicate_field("ipPrefix"));
                            }
                            ip_prefix__ = Some(map_.next_value()?);
                        }
                        GeneratedField::VendorFilter => {
                            if vendor_filter__.is_some() {
                                return Err(serde::de::Error::duplicate_field("vendorFilter"));
                            }
                            vendor_filter__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(ListAssetsRequest {
                    tenant_id: tenant_id__.unwrap_or_default(),
                    page_size: page_size__.unwrap_or_default(),
                    page_token: page_token__.unwrap_or_default(),
                    ip_prefix: ip_prefix__.unwrap_or_default(),
                    vendor_filter: vendor_filter__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.ListAssetsRequest", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for ListAssetsResponse {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.assets.is_empty() {
            len += 1;
        }
        if !self.next_page_token.is_empty() {
            len += 1;
        }
        if self.total_count != 0 {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.ListAssetsResponse", len)?;
        if !self.assets.is_empty() {
            struct_ser.serialize_field("assets", &self.assets)?;
        }
        if !self.next_page_token.is_empty() {
            struct_ser.serialize_field("nextPageToken", &self.next_page_token)?;
        }
        if self.total_count != 0 {
            struct_ser.serialize_field("totalCount", &self.total_count)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for ListAssetsResponse {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "assets",
            "next_page_token",
            "nextPageToken",
            "total_count",
            "totalCount",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Assets,
            NextPageToken,
            TotalCount,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "assets" => Ok(GeneratedField::Assets),
                            "nextPageToken" | "next_page_token" => Ok(GeneratedField::NextPageToken),
                            "totalCount" | "total_count" => Ok(GeneratedField::TotalCount),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = ListAssetsResponse;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.ListAssetsResponse")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<ListAssetsResponse, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut assets__ = None;
                let mut next_page_token__ = None;
                let mut total_count__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Assets => {
                            if assets__.is_some() {
                                return Err(serde::de::Error::duplicate_field("assets"));
                            }
                            assets__ = Some(map_.next_value()?);
                        }
                        GeneratedField::NextPageToken => {
                            if next_page_token__.is_some() {
                                return Err(serde::de::Error::duplicate_field("nextPageToken"));
                            }
                            next_page_token__ = Some(map_.next_value()?);
                        }
                        GeneratedField::TotalCount => {
                            if total_count__.is_some() {
                                return Err(serde::de::Error::duplicate_field("totalCount"));
                            }
                            total_count__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                    }
                }
                Ok(ListAssetsResponse {
                    assets: assets__.unwrap_or_default(),
                    next_page_token: next_page_token__.unwrap_or_default(),
                    total_count: total_count__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.ListAssetsResponse", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for MacIpBinding {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.mac_address.is_empty() {
            len += 1;
        }
        if !self.ip_address.is_empty() {
            len += 1;
        }
        if !self.tenant_id.is_empty() {
            len += 1;
        }
        if self.observed_at != 0 {
            len += 1;
        }
        if !self.source.is_empty() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.MacIpBinding", len)?;
        if !self.mac_address.is_empty() {
            struct_ser.serialize_field("macAddress", &self.mac_address)?;
        }
        if !self.ip_address.is_empty() {
            struct_ser.serialize_field("ipAddress", &self.ip_address)?;
        }
        if !self.tenant_id.is_empty() {
            struct_ser.serialize_field("tenantId", &self.tenant_id)?;
        }
        if self.observed_at != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("observedAt", ToString::to_string(&self.observed_at).as_str())?;
        }
        if !self.source.is_empty() {
            struct_ser.serialize_field("source", &self.source)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for MacIpBinding {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "mac_address",
            "macAddress",
            "ip_address",
            "ipAddress",
            "tenant_id",
            "tenantId",
            "observed_at",
            "observedAt",
            "source",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            MacAddress,
            IpAddress,
            TenantId,
            ObservedAt,
            Source,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "macAddress" | "mac_address" => Ok(GeneratedField::MacAddress),
                            "ipAddress" | "ip_address" => Ok(GeneratedField::IpAddress),
                            "tenantId" | "tenant_id" => Ok(GeneratedField::TenantId),
                            "observedAt" | "observed_at" => Ok(GeneratedField::ObservedAt),
                            "source" => Ok(GeneratedField::Source),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = MacIpBinding;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.MacIpBinding")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<MacIpBinding, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut mac_address__ = None;
                let mut ip_address__ = None;
                let mut tenant_id__ = None;
                let mut observed_at__ = None;
                let mut source__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::MacAddress => {
                            if mac_address__.is_some() {
                                return Err(serde::de::Error::duplicate_field("macAddress"));
                            }
                            mac_address__ = Some(map_.next_value()?);
                        }
                        GeneratedField::IpAddress => {
                            if ip_address__.is_some() {
                                return Err(serde::de::Error::duplicate_field("ipAddress"));
                            }
                            ip_address__ = Some(map_.next_value()?);
                        }
                        GeneratedField::TenantId => {
                            if tenant_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("tenantId"));
                            }
                            tenant_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::ObservedAt => {
                            if observed_at__.is_some() {
                                return Err(serde::de::Error::duplicate_field("observedAt"));
                            }
                            observed_at__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::Source => {
                            if source__.is_some() {
                                return Err(serde::de::Error::duplicate_field("source"));
                            }
                            source__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(MacIpBinding {
                    mac_address: mac_address__.unwrap_or_default(),
                    ip_address: ip_address__.unwrap_or_default(),
                    tenant_id: tenant_id__.unwrap_or_default(),
                    observed_at: observed_at__.unwrap_or_default(),
                    source: source__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.MacIpBinding", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for ModelFeedbackMetrics {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.tenant_id.is_empty() {
            len += 1;
        }
        if !self.model_version.is_empty() {
            len += 1;
        }
        if !self.alert_type.is_empty() {
            len += 1;
        }
        if self.hour != 0 {
            len += 1;
        }
        if self.total_alerts != 0 {
            len += 1;
        }
        if self.tp_count != 0 {
            len += 1;
        }
        if self.fp_count != 0 {
            len += 1;
        }
        if self.unlabeled_count != 0 {
            len += 1;
        }
        if self.precision != 0. {
            len += 1;
        }
        if self.recall != 0. {
            len += 1;
        }
        if self.f1_score != 0. {
            len += 1;
        }
        if self.ingest_ts != 0 {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.ModelFeedbackMetrics", len)?;
        if !self.tenant_id.is_empty() {
            struct_ser.serialize_field("tenantId", &self.tenant_id)?;
        }
        if !self.model_version.is_empty() {
            struct_ser.serialize_field("modelVersion", &self.model_version)?;
        }
        if !self.alert_type.is_empty() {
            struct_ser.serialize_field("alertType", &self.alert_type)?;
        }
        if self.hour != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("hour", ToString::to_string(&self.hour).as_str())?;
        }
        if self.total_alerts != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("totalAlerts", ToString::to_string(&self.total_alerts).as_str())?;
        }
        if self.tp_count != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("tpCount", ToString::to_string(&self.tp_count).as_str())?;
        }
        if self.fp_count != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("fpCount", ToString::to_string(&self.fp_count).as_str())?;
        }
        if self.unlabeled_count != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("unlabeledCount", ToString::to_string(&self.unlabeled_count).as_str())?;
        }
        if self.precision != 0. {
            struct_ser.serialize_field("precision", &self.precision)?;
        }
        if self.recall != 0. {
            struct_ser.serialize_field("recall", &self.recall)?;
        }
        if self.f1_score != 0. {
            struct_ser.serialize_field("f1Score", &self.f1_score)?;
        }
        if self.ingest_ts != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("ingestTs", ToString::to_string(&self.ingest_ts).as_str())?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for ModelFeedbackMetrics {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "tenant_id",
            "tenantId",
            "model_version",
            "modelVersion",
            "alert_type",
            "alertType",
            "hour",
            "total_alerts",
            "totalAlerts",
            "tp_count",
            "tpCount",
            "fp_count",
            "fpCount",
            "unlabeled_count",
            "unlabeledCount",
            "precision",
            "recall",
            "f1_score",
            "f1Score",
            "ingest_ts",
            "ingestTs",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            TenantId,
            ModelVersion,
            AlertType,
            Hour,
            TotalAlerts,
            TpCount,
            FpCount,
            UnlabeledCount,
            Precision,
            Recall,
            F1Score,
            IngestTs,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "tenantId" | "tenant_id" => Ok(GeneratedField::TenantId),
                            "modelVersion" | "model_version" => Ok(GeneratedField::ModelVersion),
                            "alertType" | "alert_type" => Ok(GeneratedField::AlertType),
                            "hour" => Ok(GeneratedField::Hour),
                            "totalAlerts" | "total_alerts" => Ok(GeneratedField::TotalAlerts),
                            "tpCount" | "tp_count" => Ok(GeneratedField::TpCount),
                            "fpCount" | "fp_count" => Ok(GeneratedField::FpCount),
                            "unlabeledCount" | "unlabeled_count" => Ok(GeneratedField::UnlabeledCount),
                            "precision" => Ok(GeneratedField::Precision),
                            "recall" => Ok(GeneratedField::Recall),
                            "f1Score" | "f1_score" => Ok(GeneratedField::F1Score),
                            "ingestTs" | "ingest_ts" => Ok(GeneratedField::IngestTs),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = ModelFeedbackMetrics;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.ModelFeedbackMetrics")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<ModelFeedbackMetrics, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut tenant_id__ = None;
                let mut model_version__ = None;
                let mut alert_type__ = None;
                let mut hour__ = None;
                let mut total_alerts__ = None;
                let mut tp_count__ = None;
                let mut fp_count__ = None;
                let mut unlabeled_count__ = None;
                let mut precision__ = None;
                let mut recall__ = None;
                let mut f1_score__ = None;
                let mut ingest_ts__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::TenantId => {
                            if tenant_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("tenantId"));
                            }
                            tenant_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::ModelVersion => {
                            if model_version__.is_some() {
                                return Err(serde::de::Error::duplicate_field("modelVersion"));
                            }
                            model_version__ = Some(map_.next_value()?);
                        }
                        GeneratedField::AlertType => {
                            if alert_type__.is_some() {
                                return Err(serde::de::Error::duplicate_field("alertType"));
                            }
                            alert_type__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Hour => {
                            if hour__.is_some() {
                                return Err(serde::de::Error::duplicate_field("hour"));
                            }
                            hour__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::TotalAlerts => {
                            if total_alerts__.is_some() {
                                return Err(serde::de::Error::duplicate_field("totalAlerts"));
                            }
                            total_alerts__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::TpCount => {
                            if tp_count__.is_some() {
                                return Err(serde::de::Error::duplicate_field("tpCount"));
                            }
                            tp_count__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::FpCount => {
                            if fp_count__.is_some() {
                                return Err(serde::de::Error::duplicate_field("fpCount"));
                            }
                            fp_count__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::UnlabeledCount => {
                            if unlabeled_count__.is_some() {
                                return Err(serde::de::Error::duplicate_field("unlabeledCount"));
                            }
                            unlabeled_count__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::Precision => {
                            if precision__.is_some() {
                                return Err(serde::de::Error::duplicate_field("precision"));
                            }
                            precision__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::Recall => {
                            if recall__.is_some() {
                                return Err(serde::de::Error::duplicate_field("recall"));
                            }
                            recall__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::F1Score => {
                            if f1_score__.is_some() {
                                return Err(serde::de::Error::duplicate_field("f1Score"));
                            }
                            f1_score__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::IngestTs => {
                            if ingest_ts__.is_some() {
                                return Err(serde::de::Error::duplicate_field("ingestTs"));
                            }
                            ingest_ts__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                    }
                }
                Ok(ModelFeedbackMetrics {
                    tenant_id: tenant_id__.unwrap_or_default(),
                    model_version: model_version__.unwrap_or_default(),
                    alert_type: alert_type__.unwrap_or_default(),
                    hour: hour__.unwrap_or_default(),
                    total_alerts: total_alerts__.unwrap_or_default(),
                    tp_count: tp_count__.unwrap_or_default(),
                    fp_count: fp_count__.unwrap_or_default(),
                    unlabeled_count: unlabeled_count__.unwrap_or_default(),
                    precision: precision__.unwrap_or_default(),
                    recall: recall__.unwrap_or_default(),
                    f1_score: f1_score__.unwrap_or_default(),
                    ingest_ts: ingest_ts__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.ModelFeedbackMetrics", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for Nic {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.name.is_empty() {
            len += 1;
        }
        if !self.mac_address.is_empty() {
            len += 1;
        }
        if !self.pci_address.is_empty() {
            len += 1;
        }
        if !self.driver.is_empty() {
            len += 1;
        }
        if self.speed_mbps != 0 {
            len += 1;
        }
        if !self.driver_version.is_empty() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.NIC", len)?;
        if !self.name.is_empty() {
            struct_ser.serialize_field("name", &self.name)?;
        }
        if !self.mac_address.is_empty() {
            struct_ser.serialize_field("macAddress", &self.mac_address)?;
        }
        if !self.pci_address.is_empty() {
            struct_ser.serialize_field("pciAddress", &self.pci_address)?;
        }
        if !self.driver.is_empty() {
            struct_ser.serialize_field("driver", &self.driver)?;
        }
        if self.speed_mbps != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("speedMbps", ToString::to_string(&self.speed_mbps).as_str())?;
        }
        if !self.driver_version.is_empty() {
            struct_ser.serialize_field("driverVersion", &self.driver_version)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for Nic {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "name",
            "mac_address",
            "macAddress",
            "pci_address",
            "pciAddress",
            "driver",
            "speed_mbps",
            "speedMbps",
            "driver_version",
            "driverVersion",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Name,
            MacAddress,
            PciAddress,
            Driver,
            SpeedMbps,
            DriverVersion,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "name" => Ok(GeneratedField::Name),
                            "macAddress" | "mac_address" => Ok(GeneratedField::MacAddress),
                            "pciAddress" | "pci_address" => Ok(GeneratedField::PciAddress),
                            "driver" => Ok(GeneratedField::Driver),
                            "speedMbps" | "speed_mbps" => Ok(GeneratedField::SpeedMbps),
                            "driverVersion" | "driver_version" => Ok(GeneratedField::DriverVersion),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = Nic;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.NIC")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<Nic, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut name__ = None;
                let mut mac_address__ = None;
                let mut pci_address__ = None;
                let mut driver__ = None;
                let mut speed_mbps__ = None;
                let mut driver_version__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Name => {
                            if name__.is_some() {
                                return Err(serde::de::Error::duplicate_field("name"));
                            }
                            name__ = Some(map_.next_value()?);
                        }
                        GeneratedField::MacAddress => {
                            if mac_address__.is_some() {
                                return Err(serde::de::Error::duplicate_field("macAddress"));
                            }
                            mac_address__ = Some(map_.next_value()?);
                        }
                        GeneratedField::PciAddress => {
                            if pci_address__.is_some() {
                                return Err(serde::de::Error::duplicate_field("pciAddress"));
                            }
                            pci_address__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Driver => {
                            if driver__.is_some() {
                                return Err(serde::de::Error::duplicate_field("driver"));
                            }
                            driver__ = Some(map_.next_value()?);
                        }
                        GeneratedField::SpeedMbps => {
                            if speed_mbps__.is_some() {
                                return Err(serde::de::Error::duplicate_field("speedMbps"));
                            }
                            speed_mbps__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::DriverVersion => {
                            if driver_version__.is_some() {
                                return Err(serde::de::Error::duplicate_field("driverVersion"));
                            }
                            driver_version__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(Nic {
                    name: name__.unwrap_or_default(),
                    mac_address: mac_address__.unwrap_or_default(),
                    pci_address: pci_address__.unwrap_or_default(),
                    driver: driver__.unwrap_or_default(),
                    speed_mbps: speed_mbps__.unwrap_or_default(),
                    driver_version: driver_version__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.NIC", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for NetworkInterfaceConfig {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.interface_name.is_empty() {
            len += 1;
        }
        if self.promiscuous_mode {
            len += 1;
        }
        if !self.bpf_filters.is_empty() {
            len += 1;
        }
        if self.ring_buffer_size_mb != 0 {
            len += 1;
        }
        if !self.driver_mode.is_empty() {
            len += 1;
        }
        if self.cpu_affinity.is_some() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.NetworkInterfaceConfig", len)?;
        if !self.interface_name.is_empty() {
            struct_ser.serialize_field("interfaceName", &self.interface_name)?;
        }
        if self.promiscuous_mode {
            struct_ser.serialize_field("promiscuousMode", &self.promiscuous_mode)?;
        }
        if !self.bpf_filters.is_empty() {
            struct_ser.serialize_field("bpfFilters", &self.bpf_filters)?;
        }
        if self.ring_buffer_size_mb != 0 {
            struct_ser.serialize_field("ringBufferSizeMb", &self.ring_buffer_size_mb)?;
        }
        if !self.driver_mode.is_empty() {
            struct_ser.serialize_field("driverMode", &self.driver_mode)?;
        }
        if let Some(v) = self.cpu_affinity.as_ref() {
            struct_ser.serialize_field("cpuAffinity", v)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for NetworkInterfaceConfig {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "interface_name",
            "interfaceName",
            "promiscuous_mode",
            "promiscuousMode",
            "bpf_filters",
            "bpfFilters",
            "ring_buffer_size_mb",
            "ringBufferSizeMb",
            "driver_mode",
            "driverMode",
            "cpu_affinity",
            "cpuAffinity",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            InterfaceName,
            PromiscuousMode,
            BpfFilters,
            RingBufferSizeMb,
            DriverMode,
            CpuAffinity,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "interfaceName" | "interface_name" => Ok(GeneratedField::InterfaceName),
                            "promiscuousMode" | "promiscuous_mode" => Ok(GeneratedField::PromiscuousMode),
                            "bpfFilters" | "bpf_filters" => Ok(GeneratedField::BpfFilters),
                            "ringBufferSizeMb" | "ring_buffer_size_mb" => Ok(GeneratedField::RingBufferSizeMb),
                            "driverMode" | "driver_mode" => Ok(GeneratedField::DriverMode),
                            "cpuAffinity" | "cpu_affinity" => Ok(GeneratedField::CpuAffinity),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = NetworkInterfaceConfig;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.NetworkInterfaceConfig")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<NetworkInterfaceConfig, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut interface_name__ = None;
                let mut promiscuous_mode__ = None;
                let mut bpf_filters__ = None;
                let mut ring_buffer_size_mb__ = None;
                let mut driver_mode__ = None;
                let mut cpu_affinity__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::InterfaceName => {
                            if interface_name__.is_some() {
                                return Err(serde::de::Error::duplicate_field("interfaceName"));
                            }
                            interface_name__ = Some(map_.next_value()?);
                        }
                        GeneratedField::PromiscuousMode => {
                            if promiscuous_mode__.is_some() {
                                return Err(serde::de::Error::duplicate_field("promiscuousMode"));
                            }
                            promiscuous_mode__ = Some(map_.next_value()?);
                        }
                        GeneratedField::BpfFilters => {
                            if bpf_filters__.is_some() {
                                return Err(serde::de::Error::duplicate_field("bpfFilters"));
                            }
                            bpf_filters__ = Some(map_.next_value()?);
                        }
                        GeneratedField::RingBufferSizeMb => {
                            if ring_buffer_size_mb__.is_some() {
                                return Err(serde::de::Error::duplicate_field("ringBufferSizeMb"));
                            }
                            ring_buffer_size_mb__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::DriverMode => {
                            if driver_mode__.is_some() {
                                return Err(serde::de::Error::duplicate_field("driverMode"));
                            }
                            driver_mode__ = Some(map_.next_value()?);
                        }
                        GeneratedField::CpuAffinity => {
                            if cpu_affinity__.is_some() {
                                return Err(serde::de::Error::duplicate_field("cpuAffinity"));
                            }
                            cpu_affinity__ = map_.next_value()?;
                        }
                    }
                }
                Ok(NetworkInterfaceConfig {
                    interface_name: interface_name__.unwrap_or_default(),
                    promiscuous_mode: promiscuous_mode__.unwrap_or_default(),
                    bpf_filters: bpf_filters__.unwrap_or_default(),
                    ring_buffer_size_mb: ring_buffer_size_mb__.unwrap_or_default(),
                    driver_mode: driver_mode__.unwrap_or_default(),
                    cpu_affinity: cpu_affinity__,
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.NetworkInterfaceConfig", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for NotificationEvent {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.tenant_id.is_empty() {
            len += 1;
        }
        if !self.notification_id.is_empty() {
            len += 1;
        }
        if !self.alert_id.is_empty() {
            len += 1;
        }
        if !self.channel.is_empty() {
            len += 1;
        }
        if !self.status.is_empty() {
            len += 1;
        }
        if !self.error_message.is_empty() {
            len += 1;
        }
        if !self.rule_id.is_empty() {
            len += 1;
        }
        if !self.recipient.is_empty() {
            len += 1;
        }
        if self.sent_at != 0 {
            len += 1;
        }
        if self.ingest_ts != 0 {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.NotificationEvent", len)?;
        if !self.tenant_id.is_empty() {
            struct_ser.serialize_field("tenantId", &self.tenant_id)?;
        }
        if !self.notification_id.is_empty() {
            struct_ser.serialize_field("notificationId", &self.notification_id)?;
        }
        if !self.alert_id.is_empty() {
            struct_ser.serialize_field("alertId", &self.alert_id)?;
        }
        if !self.channel.is_empty() {
            struct_ser.serialize_field("channel", &self.channel)?;
        }
        if !self.status.is_empty() {
            struct_ser.serialize_field("status", &self.status)?;
        }
        if !self.error_message.is_empty() {
            struct_ser.serialize_field("errorMessage", &self.error_message)?;
        }
        if !self.rule_id.is_empty() {
            struct_ser.serialize_field("ruleId", &self.rule_id)?;
        }
        if !self.recipient.is_empty() {
            struct_ser.serialize_field("recipient", &self.recipient)?;
        }
        if self.sent_at != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("sentAt", ToString::to_string(&self.sent_at).as_str())?;
        }
        if self.ingest_ts != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("ingestTs", ToString::to_string(&self.ingest_ts).as_str())?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for NotificationEvent {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "tenant_id",
            "tenantId",
            "notification_id",
            "notificationId",
            "alert_id",
            "alertId",
            "channel",
            "status",
            "error_message",
            "errorMessage",
            "rule_id",
            "ruleId",
            "recipient",
            "sent_at",
            "sentAt",
            "ingest_ts",
            "ingestTs",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            TenantId,
            NotificationId,
            AlertId,
            Channel,
            Status,
            ErrorMessage,
            RuleId,
            Recipient,
            SentAt,
            IngestTs,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "tenantId" | "tenant_id" => Ok(GeneratedField::TenantId),
                            "notificationId" | "notification_id" => Ok(GeneratedField::NotificationId),
                            "alertId" | "alert_id" => Ok(GeneratedField::AlertId),
                            "channel" => Ok(GeneratedField::Channel),
                            "status" => Ok(GeneratedField::Status),
                            "errorMessage" | "error_message" => Ok(GeneratedField::ErrorMessage),
                            "ruleId" | "rule_id" => Ok(GeneratedField::RuleId),
                            "recipient" => Ok(GeneratedField::Recipient),
                            "sentAt" | "sent_at" => Ok(GeneratedField::SentAt),
                            "ingestTs" | "ingest_ts" => Ok(GeneratedField::IngestTs),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = NotificationEvent;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.NotificationEvent")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<NotificationEvent, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut tenant_id__ = None;
                let mut notification_id__ = None;
                let mut alert_id__ = None;
                let mut channel__ = None;
                let mut status__ = None;
                let mut error_message__ = None;
                let mut rule_id__ = None;
                let mut recipient__ = None;
                let mut sent_at__ = None;
                let mut ingest_ts__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::TenantId => {
                            if tenant_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("tenantId"));
                            }
                            tenant_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::NotificationId => {
                            if notification_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("notificationId"));
                            }
                            notification_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::AlertId => {
                            if alert_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("alertId"));
                            }
                            alert_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Channel => {
                            if channel__.is_some() {
                                return Err(serde::de::Error::duplicate_field("channel"));
                            }
                            channel__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Status => {
                            if status__.is_some() {
                                return Err(serde::de::Error::duplicate_field("status"));
                            }
                            status__ = Some(map_.next_value()?);
                        }
                        GeneratedField::ErrorMessage => {
                            if error_message__.is_some() {
                                return Err(serde::de::Error::duplicate_field("errorMessage"));
                            }
                            error_message__ = Some(map_.next_value()?);
                        }
                        GeneratedField::RuleId => {
                            if rule_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("ruleId"));
                            }
                            rule_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Recipient => {
                            if recipient__.is_some() {
                                return Err(serde::de::Error::duplicate_field("recipient"));
                            }
                            recipient__ = Some(map_.next_value()?);
                        }
                        GeneratedField::SentAt => {
                            if sent_at__.is_some() {
                                return Err(serde::de::Error::duplicate_field("sentAt"));
                            }
                            sent_at__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::IngestTs => {
                            if ingest_ts__.is_some() {
                                return Err(serde::de::Error::duplicate_field("ingestTs"));
                            }
                            ingest_ts__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                    }
                }
                Ok(NotificationEvent {
                    tenant_id: tenant_id__.unwrap_or_default(),
                    notification_id: notification_id__.unwrap_or_default(),
                    alert_id: alert_id__.unwrap_or_default(),
                    channel: channel__.unwrap_or_default(),
                    status: status__.unwrap_or_default(),
                    error_message: error_message__.unwrap_or_default(),
                    rule_id: rule_id__.unwrap_or_default(),
                    recipient: recipient__.unwrap_or_default(),
                    sent_at: sent_at__.unwrap_or_default(),
                    ingest_ts: ingest_ts__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.NotificationEvent", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for PacketLengthStats {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if self.min != 0 {
            len += 1;
        }
        if self.max != 0 {
            len += 1;
        }
        if self.mean != 0. {
            len += 1;
        }
        if self.std != 0. {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.PacketLengthStats", len)?;
        if self.min != 0 {
            struct_ser.serialize_field("min", &self.min)?;
        }
        if self.max != 0 {
            struct_ser.serialize_field("max", &self.max)?;
        }
        if self.mean != 0. {
            struct_ser.serialize_field("mean", &self.mean)?;
        }
        if self.std != 0. {
            struct_ser.serialize_field("std", &self.std)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for PacketLengthStats {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "min",
            "max",
            "mean",
            "std",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Min,
            Max,
            Mean,
            Std,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "min" => Ok(GeneratedField::Min),
                            "max" => Ok(GeneratedField::Max),
                            "mean" => Ok(GeneratedField::Mean),
                            "std" => Ok(GeneratedField::Std),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = PacketLengthStats;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.PacketLengthStats")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<PacketLengthStats, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut min__ = None;
                let mut max__ = None;
                let mut mean__ = None;
                let mut std__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Min => {
                            if min__.is_some() {
                                return Err(serde::de::Error::duplicate_field("min"));
                            }
                            min__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::Max => {
                            if max__.is_some() {
                                return Err(serde::de::Error::duplicate_field("max"));
                            }
                            max__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::Mean => {
                            if mean__.is_some() {
                                return Err(serde::de::Error::duplicate_field("mean"));
                            }
                            mean__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::Std => {
                            if std__.is_some() {
                                return Err(serde::de::Error::duplicate_field("std"));
                            }
                            std__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                    }
                }
                Ok(PacketLengthStats {
                    min: min__.unwrap_or_default(),
                    max: max__.unwrap_or_default(),
                    mean: mean__.unwrap_or_default(),
                    std: std__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.PacketLengthStats", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for PcapCutJobStatus {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.job_id.is_empty() {
            len += 1;
        }
        if !self.tenant_id.is_empty() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.PcapCutJobStatus", len)?;
        if !self.job_id.is_empty() {
            struct_ser.serialize_field("jobId", &self.job_id)?;
        }
        if !self.tenant_id.is_empty() {
            struct_ser.serialize_field("tenantId", &self.tenant_id)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for PcapCutJobStatus {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "job_id",
            "jobId",
            "tenant_id",
            "tenantId",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            JobId,
            TenantId,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "jobId" | "job_id" => Ok(GeneratedField::JobId),
                            "tenantId" | "tenant_id" => Ok(GeneratedField::TenantId),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = PcapCutJobStatus;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.PcapCutJobStatus")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<PcapCutJobStatus, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut job_id__ = None;
                let mut tenant_id__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::JobId => {
                            if job_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("jobId"));
                            }
                            job_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::TenantId => {
                            if tenant_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("tenantId"));
                            }
                            tenant_id__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(PcapCutJobStatus {
                    job_id: job_id__.unwrap_or_default(),
                    tenant_id: tenant_id__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.PcapCutJobStatus", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for PcapCutRequest {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.tenant_id.is_empty() {
            len += 1;
        }
        if !self.src_ip.is_empty() {
            len += 1;
        }
        if !self.dst_ip.is_empty() {
            len += 1;
        }
        if self.src_port != 0 {
            len += 1;
        }
        if self.dst_port != 0 {
            len += 1;
        }
        if self.protocol != 0 {
            len += 1;
        }
        if self.start_time != 0 {
            len += 1;
        }
        if self.end_time != 0 {
            len += 1;
        }
        if !self.community_id.is_empty() {
            len += 1;
        }
        if !self.flow_id.is_empty() {
            len += 1;
        }
        if self.max_packets != 0 {
            len += 1;
        }
        if self.max_bytes != 0 {
            len += 1;
        }
        if !self.output_format.is_empty() {
            len += 1;
        }
        if self.compress {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.PcapCutRequest", len)?;
        if !self.tenant_id.is_empty() {
            struct_ser.serialize_field("tenantId", &self.tenant_id)?;
        }
        if !self.src_ip.is_empty() {
            struct_ser.serialize_field("srcIp", &self.src_ip)?;
        }
        if !self.dst_ip.is_empty() {
            struct_ser.serialize_field("dstIp", &self.dst_ip)?;
        }
        if self.src_port != 0 {
            struct_ser.serialize_field("srcPort", &self.src_port)?;
        }
        if self.dst_port != 0 {
            struct_ser.serialize_field("dstPort", &self.dst_port)?;
        }
        if self.protocol != 0 {
            struct_ser.serialize_field("protocol", &self.protocol)?;
        }
        if self.start_time != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("startTime", ToString::to_string(&self.start_time).as_str())?;
        }
        if self.end_time != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("endTime", ToString::to_string(&self.end_time).as_str())?;
        }
        if !self.community_id.is_empty() {
            struct_ser.serialize_field("communityId", &self.community_id)?;
        }
        if !self.flow_id.is_empty() {
            struct_ser.serialize_field("flowId", &self.flow_id)?;
        }
        if self.max_packets != 0 {
            struct_ser.serialize_field("maxPackets", &self.max_packets)?;
        }
        if self.max_bytes != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("maxBytes", ToString::to_string(&self.max_bytes).as_str())?;
        }
        if !self.output_format.is_empty() {
            struct_ser.serialize_field("outputFormat", &self.output_format)?;
        }
        if self.compress {
            struct_ser.serialize_field("compress", &self.compress)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for PcapCutRequest {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "tenant_id",
            "tenantId",
            "src_ip",
            "srcIp",
            "dst_ip",
            "dstIp",
            "src_port",
            "srcPort",
            "dst_port",
            "dstPort",
            "protocol",
            "start_time",
            "startTime",
            "end_time",
            "endTime",
            "community_id",
            "communityId",
            "flow_id",
            "flowId",
            "max_packets",
            "maxPackets",
            "max_bytes",
            "maxBytes",
            "output_format",
            "outputFormat",
            "compress",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            TenantId,
            SrcIp,
            DstIp,
            SrcPort,
            DstPort,
            Protocol,
            StartTime,
            EndTime,
            CommunityId,
            FlowId,
            MaxPackets,
            MaxBytes,
            OutputFormat,
            Compress,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "tenantId" | "tenant_id" => Ok(GeneratedField::TenantId),
                            "srcIp" | "src_ip" => Ok(GeneratedField::SrcIp),
                            "dstIp" | "dst_ip" => Ok(GeneratedField::DstIp),
                            "srcPort" | "src_port" => Ok(GeneratedField::SrcPort),
                            "dstPort" | "dst_port" => Ok(GeneratedField::DstPort),
                            "protocol" => Ok(GeneratedField::Protocol),
                            "startTime" | "start_time" => Ok(GeneratedField::StartTime),
                            "endTime" | "end_time" => Ok(GeneratedField::EndTime),
                            "communityId" | "community_id" => Ok(GeneratedField::CommunityId),
                            "flowId" | "flow_id" => Ok(GeneratedField::FlowId),
                            "maxPackets" | "max_packets" => Ok(GeneratedField::MaxPackets),
                            "maxBytes" | "max_bytes" => Ok(GeneratedField::MaxBytes),
                            "outputFormat" | "output_format" => Ok(GeneratedField::OutputFormat),
                            "compress" => Ok(GeneratedField::Compress),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = PcapCutRequest;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.PcapCutRequest")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<PcapCutRequest, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut tenant_id__ = None;
                let mut src_ip__ = None;
                let mut dst_ip__ = None;
                let mut src_port__ = None;
                let mut dst_port__ = None;
                let mut protocol__ = None;
                let mut start_time__ = None;
                let mut end_time__ = None;
                let mut community_id__ = None;
                let mut flow_id__ = None;
                let mut max_packets__ = None;
                let mut max_bytes__ = None;
                let mut output_format__ = None;
                let mut compress__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::TenantId => {
                            if tenant_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("tenantId"));
                            }
                            tenant_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::SrcIp => {
                            if src_ip__.is_some() {
                                return Err(serde::de::Error::duplicate_field("srcIp"));
                            }
                            src_ip__ = Some(map_.next_value()?);
                        }
                        GeneratedField::DstIp => {
                            if dst_ip__.is_some() {
                                return Err(serde::de::Error::duplicate_field("dstIp"));
                            }
                            dst_ip__ = Some(map_.next_value()?);
                        }
                        GeneratedField::SrcPort => {
                            if src_port__.is_some() {
                                return Err(serde::de::Error::duplicate_field("srcPort"));
                            }
                            src_port__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::DstPort => {
                            if dst_port__.is_some() {
                                return Err(serde::de::Error::duplicate_field("dstPort"));
                            }
                            dst_port__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::Protocol => {
                            if protocol__.is_some() {
                                return Err(serde::de::Error::duplicate_field("protocol"));
                            }
                            protocol__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::StartTime => {
                            if start_time__.is_some() {
                                return Err(serde::de::Error::duplicate_field("startTime"));
                            }
                            start_time__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::EndTime => {
                            if end_time__.is_some() {
                                return Err(serde::de::Error::duplicate_field("endTime"));
                            }
                            end_time__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::CommunityId => {
                            if community_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("communityId"));
                            }
                            community_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::FlowId => {
                            if flow_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("flowId"));
                            }
                            flow_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::MaxPackets => {
                            if max_packets__.is_some() {
                                return Err(serde::de::Error::duplicate_field("maxPackets"));
                            }
                            max_packets__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::MaxBytes => {
                            if max_bytes__.is_some() {
                                return Err(serde::de::Error::duplicate_field("maxBytes"));
                            }
                            max_bytes__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::OutputFormat => {
                            if output_format__.is_some() {
                                return Err(serde::de::Error::duplicate_field("outputFormat"));
                            }
                            output_format__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Compress => {
                            if compress__.is_some() {
                                return Err(serde::de::Error::duplicate_field("compress"));
                            }
                            compress__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(PcapCutRequest {
                    tenant_id: tenant_id__.unwrap_or_default(),
                    src_ip: src_ip__.unwrap_or_default(),
                    dst_ip: dst_ip__.unwrap_or_default(),
                    src_port: src_port__.unwrap_or_default(),
                    dst_port: dst_port__.unwrap_or_default(),
                    protocol: protocol__.unwrap_or_default(),
                    start_time: start_time__.unwrap_or_default(),
                    end_time: end_time__.unwrap_or_default(),
                    community_id: community_id__.unwrap_or_default(),
                    flow_id: flow_id__.unwrap_or_default(),
                    max_packets: max_packets__.unwrap_or_default(),
                    max_bytes: max_bytes__.unwrap_or_default(),
                    output_format: output_format__.unwrap_or_default(),
                    compress: compress__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.PcapCutRequest", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for PcapCutResponse {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.job_id.is_empty() {
            len += 1;
        }
        if !self.status.is_empty() {
            len += 1;
        }
        if !self.download_url.is_empty() {
            len += 1;
        }
        if self.progress_percent != 0 {
            len += 1;
        }
        if !self.error_message.is_empty() {
            len += 1;
        }
        if self.total_packets != 0 {
            len += 1;
        }
        if self.total_bytes != 0 {
            len += 1;
        }
        if self.files_scanned != 0 {
            len += 1;
        }
        if self.files_matched != 0 {
            len += 1;
        }
        if self.created_at != 0 {
            len += 1;
        }
        if self.started_at != 0 {
            len += 1;
        }
        if self.completed_at != 0 {
            len += 1;
        }
        if self.expires_at != 0 {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.PcapCutResponse", len)?;
        if !self.job_id.is_empty() {
            struct_ser.serialize_field("jobId", &self.job_id)?;
        }
        if !self.status.is_empty() {
            struct_ser.serialize_field("status", &self.status)?;
        }
        if !self.download_url.is_empty() {
            struct_ser.serialize_field("downloadUrl", &self.download_url)?;
        }
        if self.progress_percent != 0 {
            struct_ser.serialize_field("progressPercent", &self.progress_percent)?;
        }
        if !self.error_message.is_empty() {
            struct_ser.serialize_field("errorMessage", &self.error_message)?;
        }
        if self.total_packets != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("totalPackets", ToString::to_string(&self.total_packets).as_str())?;
        }
        if self.total_bytes != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("totalBytes", ToString::to_string(&self.total_bytes).as_str())?;
        }
        if self.files_scanned != 0 {
            struct_ser.serialize_field("filesScanned", &self.files_scanned)?;
        }
        if self.files_matched != 0 {
            struct_ser.serialize_field("filesMatched", &self.files_matched)?;
        }
        if self.created_at != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("createdAt", ToString::to_string(&self.created_at).as_str())?;
        }
        if self.started_at != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("startedAt", ToString::to_string(&self.started_at).as_str())?;
        }
        if self.completed_at != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("completedAt", ToString::to_string(&self.completed_at).as_str())?;
        }
        if self.expires_at != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("expiresAt", ToString::to_string(&self.expires_at).as_str())?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for PcapCutResponse {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "job_id",
            "jobId",
            "status",
            "download_url",
            "downloadUrl",
            "progress_percent",
            "progressPercent",
            "error_message",
            "errorMessage",
            "total_packets",
            "totalPackets",
            "total_bytes",
            "totalBytes",
            "files_scanned",
            "filesScanned",
            "files_matched",
            "filesMatched",
            "created_at",
            "createdAt",
            "started_at",
            "startedAt",
            "completed_at",
            "completedAt",
            "expires_at",
            "expiresAt",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            JobId,
            Status,
            DownloadUrl,
            ProgressPercent,
            ErrorMessage,
            TotalPackets,
            TotalBytes,
            FilesScanned,
            FilesMatched,
            CreatedAt,
            StartedAt,
            CompletedAt,
            ExpiresAt,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "jobId" | "job_id" => Ok(GeneratedField::JobId),
                            "status" => Ok(GeneratedField::Status),
                            "downloadUrl" | "download_url" => Ok(GeneratedField::DownloadUrl),
                            "progressPercent" | "progress_percent" => Ok(GeneratedField::ProgressPercent),
                            "errorMessage" | "error_message" => Ok(GeneratedField::ErrorMessage),
                            "totalPackets" | "total_packets" => Ok(GeneratedField::TotalPackets),
                            "totalBytes" | "total_bytes" => Ok(GeneratedField::TotalBytes),
                            "filesScanned" | "files_scanned" => Ok(GeneratedField::FilesScanned),
                            "filesMatched" | "files_matched" => Ok(GeneratedField::FilesMatched),
                            "createdAt" | "created_at" => Ok(GeneratedField::CreatedAt),
                            "startedAt" | "started_at" => Ok(GeneratedField::StartedAt),
                            "completedAt" | "completed_at" => Ok(GeneratedField::CompletedAt),
                            "expiresAt" | "expires_at" => Ok(GeneratedField::ExpiresAt),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = PcapCutResponse;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.PcapCutResponse")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<PcapCutResponse, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut job_id__ = None;
                let mut status__ = None;
                let mut download_url__ = None;
                let mut progress_percent__ = None;
                let mut error_message__ = None;
                let mut total_packets__ = None;
                let mut total_bytes__ = None;
                let mut files_scanned__ = None;
                let mut files_matched__ = None;
                let mut created_at__ = None;
                let mut started_at__ = None;
                let mut completed_at__ = None;
                let mut expires_at__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::JobId => {
                            if job_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("jobId"));
                            }
                            job_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Status => {
                            if status__.is_some() {
                                return Err(serde::de::Error::duplicate_field("status"));
                            }
                            status__ = Some(map_.next_value()?);
                        }
                        GeneratedField::DownloadUrl => {
                            if download_url__.is_some() {
                                return Err(serde::de::Error::duplicate_field("downloadUrl"));
                            }
                            download_url__ = Some(map_.next_value()?);
                        }
                        GeneratedField::ProgressPercent => {
                            if progress_percent__.is_some() {
                                return Err(serde::de::Error::duplicate_field("progressPercent"));
                            }
                            progress_percent__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::ErrorMessage => {
                            if error_message__.is_some() {
                                return Err(serde::de::Error::duplicate_field("errorMessage"));
                            }
                            error_message__ = Some(map_.next_value()?);
                        }
                        GeneratedField::TotalPackets => {
                            if total_packets__.is_some() {
                                return Err(serde::de::Error::duplicate_field("totalPackets"));
                            }
                            total_packets__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::TotalBytes => {
                            if total_bytes__.is_some() {
                                return Err(serde::de::Error::duplicate_field("totalBytes"));
                            }
                            total_bytes__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::FilesScanned => {
                            if files_scanned__.is_some() {
                                return Err(serde::de::Error::duplicate_field("filesScanned"));
                            }
                            files_scanned__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::FilesMatched => {
                            if files_matched__.is_some() {
                                return Err(serde::de::Error::duplicate_field("filesMatched"));
                            }
                            files_matched__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::CreatedAt => {
                            if created_at__.is_some() {
                                return Err(serde::de::Error::duplicate_field("createdAt"));
                            }
                            created_at__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::StartedAt => {
                            if started_at__.is_some() {
                                return Err(serde::de::Error::duplicate_field("startedAt"));
                            }
                            started_at__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::CompletedAt => {
                            if completed_at__.is_some() {
                                return Err(serde::de::Error::duplicate_field("completedAt"));
                            }
                            completed_at__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::ExpiresAt => {
                            if expires_at__.is_some() {
                                return Err(serde::de::Error::duplicate_field("expiresAt"));
                            }
                            expires_at__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                    }
                }
                Ok(PcapCutResponse {
                    job_id: job_id__.unwrap_or_default(),
                    status: status__.unwrap_or_default(),
                    download_url: download_url__.unwrap_or_default(),
                    progress_percent: progress_percent__.unwrap_or_default(),
                    error_message: error_message__.unwrap_or_default(),
                    total_packets: total_packets__.unwrap_or_default(),
                    total_bytes: total_bytes__.unwrap_or_default(),
                    files_scanned: files_scanned__.unwrap_or_default(),
                    files_matched: files_matched__.unwrap_or_default(),
                    created_at: created_at__.unwrap_or_default(),
                    started_at: started_at__.unwrap_or_default(),
                    completed_at: completed_at__.unwrap_or_default(),
                    expires_at: expires_at__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.PcapCutResponse", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for PcapIndexBatch {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.indexes.is_empty() {
            len += 1;
        }
        if !self.batch_id.is_empty() {
            len += 1;
        }
        if !self.tenant_id.is_empty() {
            len += 1;
        }
        if !self.probe_id.is_empty() {
            len += 1;
        }
        if self.created_at != 0 {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.PcapIndexBatch", len)?;
        if !self.indexes.is_empty() {
            struct_ser.serialize_field("indexes", &self.indexes)?;
        }
        if !self.batch_id.is_empty() {
            struct_ser.serialize_field("batchId", &self.batch_id)?;
        }
        if !self.tenant_id.is_empty() {
            struct_ser.serialize_field("tenantId", &self.tenant_id)?;
        }
        if !self.probe_id.is_empty() {
            struct_ser.serialize_field("probeId", &self.probe_id)?;
        }
        if self.created_at != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("createdAt", ToString::to_string(&self.created_at).as_str())?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for PcapIndexBatch {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "indexes",
            "batch_id",
            "batchId",
            "tenant_id",
            "tenantId",
            "probe_id",
            "probeId",
            "created_at",
            "createdAt",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Indexes,
            BatchId,
            TenantId,
            ProbeId,
            CreatedAt,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "indexes" => Ok(GeneratedField::Indexes),
                            "batchId" | "batch_id" => Ok(GeneratedField::BatchId),
                            "tenantId" | "tenant_id" => Ok(GeneratedField::TenantId),
                            "probeId" | "probe_id" => Ok(GeneratedField::ProbeId),
                            "createdAt" | "created_at" => Ok(GeneratedField::CreatedAt),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = PcapIndexBatch;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.PcapIndexBatch")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<PcapIndexBatch, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut indexes__ = None;
                let mut batch_id__ = None;
                let mut tenant_id__ = None;
                let mut probe_id__ = None;
                let mut created_at__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Indexes => {
                            if indexes__.is_some() {
                                return Err(serde::de::Error::duplicate_field("indexes"));
                            }
                            indexes__ = Some(map_.next_value()?);
                        }
                        GeneratedField::BatchId => {
                            if batch_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("batchId"));
                            }
                            batch_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::TenantId => {
                            if tenant_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("tenantId"));
                            }
                            tenant_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::ProbeId => {
                            if probe_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("probeId"));
                            }
                            probe_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::CreatedAt => {
                            if created_at__.is_some() {
                                return Err(serde::de::Error::duplicate_field("createdAt"));
                            }
                            created_at__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                    }
                }
                Ok(PcapIndexBatch {
                    indexes: indexes__.unwrap_or_default(),
                    batch_id: batch_id__.unwrap_or_default(),
                    tenant_id: tenant_id__.unwrap_or_default(),
                    probe_id: probe_id__.unwrap_or_default(),
                    created_at: created_at__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.PcapIndexBatch", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for PcapIndexMeta {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.tenant_id.is_empty() {
            len += 1;
        }
        if !self.probe_id.is_empty() {
            len += 1;
        }
        if !self.file_key.is_empty() {
            len += 1;
        }
        if self.ts_start != 0 {
            len += 1;
        }
        if self.ts_end != 0 {
            len += 1;
        }
        if self.byte_size != 0 {
            len += 1;
        }
        if self.zstd_level != 0 {
            len += 1;
        }
        if !self.sha256.is_empty() {
            len += 1;
        }
        if !self.community_id.is_empty() {
            len += 1;
        }
        if !self.flow_id.is_empty() {
            len += 1;
        }
        if self.offset_start != 0 {
            len += 1;
        }
        if self.offset_end != 0 {
            len += 1;
        }
        if !self.bloom_filter_b64.is_empty() {
            len += 1;
        }
        if !self.community_ids.is_empty() {
            len += 1;
        }
        if self.created_ts != 0 {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.PcapIndexMeta", len)?;
        if !self.tenant_id.is_empty() {
            struct_ser.serialize_field("tenantId", &self.tenant_id)?;
        }
        if !self.probe_id.is_empty() {
            struct_ser.serialize_field("probeId", &self.probe_id)?;
        }
        if !self.file_key.is_empty() {
            struct_ser.serialize_field("fileKey", &self.file_key)?;
        }
        if self.ts_start != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("tsStart", ToString::to_string(&self.ts_start).as_str())?;
        }
        if self.ts_end != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("tsEnd", ToString::to_string(&self.ts_end).as_str())?;
        }
        if self.byte_size != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("byteSize", ToString::to_string(&self.byte_size).as_str())?;
        }
        if self.zstd_level != 0 {
            struct_ser.serialize_field("zstdLevel", &self.zstd_level)?;
        }
        if !self.sha256.is_empty() {
            struct_ser.serialize_field("sha256", &self.sha256)?;
        }
        if !self.community_id.is_empty() {
            struct_ser.serialize_field("communityId", &self.community_id)?;
        }
        if !self.flow_id.is_empty() {
            struct_ser.serialize_field("flowId", &self.flow_id)?;
        }
        if self.offset_start != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("offsetStart", ToString::to_string(&self.offset_start).as_str())?;
        }
        if self.offset_end != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("offsetEnd", ToString::to_string(&self.offset_end).as_str())?;
        }
        if !self.bloom_filter_b64.is_empty() {
            struct_ser.serialize_field("bloomFilterB64", &self.bloom_filter_b64)?;
        }
        if !self.community_ids.is_empty() {
            struct_ser.serialize_field("communityIds", &self.community_ids)?;
        }
        if self.created_ts != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("createdTs", ToString::to_string(&self.created_ts).as_str())?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for PcapIndexMeta {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "tenant_id",
            "tenantId",
            "probe_id",
            "probeId",
            "file_key",
            "fileKey",
            "ts_start",
            "tsStart",
            "ts_end",
            "tsEnd",
            "byte_size",
            "byteSize",
            "zstd_level",
            "zstdLevel",
            "sha256",
            "community_id",
            "communityId",
            "flow_id",
            "flowId",
            "offset_start",
            "offsetStart",
            "offset_end",
            "offsetEnd",
            "bloom_filter_b64",
            "bloomFilterB64",
            "community_ids",
            "communityIds",
            "created_ts",
            "createdTs",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            TenantId,
            ProbeId,
            FileKey,
            TsStart,
            TsEnd,
            ByteSize,
            ZstdLevel,
            Sha256,
            CommunityId,
            FlowId,
            OffsetStart,
            OffsetEnd,
            BloomFilterB64,
            CommunityIds,
            CreatedTs,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "tenantId" | "tenant_id" => Ok(GeneratedField::TenantId),
                            "probeId" | "probe_id" => Ok(GeneratedField::ProbeId),
                            "fileKey" | "file_key" => Ok(GeneratedField::FileKey),
                            "tsStart" | "ts_start" => Ok(GeneratedField::TsStart),
                            "tsEnd" | "ts_end" => Ok(GeneratedField::TsEnd),
                            "byteSize" | "byte_size" => Ok(GeneratedField::ByteSize),
                            "zstdLevel" | "zstd_level" => Ok(GeneratedField::ZstdLevel),
                            "sha256" => Ok(GeneratedField::Sha256),
                            "communityId" | "community_id" => Ok(GeneratedField::CommunityId),
                            "flowId" | "flow_id" => Ok(GeneratedField::FlowId),
                            "offsetStart" | "offset_start" => Ok(GeneratedField::OffsetStart),
                            "offsetEnd" | "offset_end" => Ok(GeneratedField::OffsetEnd),
                            "bloomFilterB64" | "bloom_filter_b64" => Ok(GeneratedField::BloomFilterB64),
                            "communityIds" | "community_ids" => Ok(GeneratedField::CommunityIds),
                            "createdTs" | "created_ts" => Ok(GeneratedField::CreatedTs),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = PcapIndexMeta;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.PcapIndexMeta")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<PcapIndexMeta, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut tenant_id__ = None;
                let mut probe_id__ = None;
                let mut file_key__ = None;
                let mut ts_start__ = None;
                let mut ts_end__ = None;
                let mut byte_size__ = None;
                let mut zstd_level__ = None;
                let mut sha256__ = None;
                let mut community_id__ = None;
                let mut flow_id__ = None;
                let mut offset_start__ = None;
                let mut offset_end__ = None;
                let mut bloom_filter_b64__ = None;
                let mut community_ids__ = None;
                let mut created_ts__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::TenantId => {
                            if tenant_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("tenantId"));
                            }
                            tenant_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::ProbeId => {
                            if probe_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("probeId"));
                            }
                            probe_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::FileKey => {
                            if file_key__.is_some() {
                                return Err(serde::de::Error::duplicate_field("fileKey"));
                            }
                            file_key__ = Some(map_.next_value()?);
                        }
                        GeneratedField::TsStart => {
                            if ts_start__.is_some() {
                                return Err(serde::de::Error::duplicate_field("tsStart"));
                            }
                            ts_start__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::TsEnd => {
                            if ts_end__.is_some() {
                                return Err(serde::de::Error::duplicate_field("tsEnd"));
                            }
                            ts_end__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::ByteSize => {
                            if byte_size__.is_some() {
                                return Err(serde::de::Error::duplicate_field("byteSize"));
                            }
                            byte_size__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::ZstdLevel => {
                            if zstd_level__.is_some() {
                                return Err(serde::de::Error::duplicate_field("zstdLevel"));
                            }
                            zstd_level__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::Sha256 => {
                            if sha256__.is_some() {
                                return Err(serde::de::Error::duplicate_field("sha256"));
                            }
                            sha256__ = Some(map_.next_value()?);
                        }
                        GeneratedField::CommunityId => {
                            if community_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("communityId"));
                            }
                            community_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::FlowId => {
                            if flow_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("flowId"));
                            }
                            flow_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::OffsetStart => {
                            if offset_start__.is_some() {
                                return Err(serde::de::Error::duplicate_field("offsetStart"));
                            }
                            offset_start__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::OffsetEnd => {
                            if offset_end__.is_some() {
                                return Err(serde::de::Error::duplicate_field("offsetEnd"));
                            }
                            offset_end__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::BloomFilterB64 => {
                            if bloom_filter_b64__.is_some() {
                                return Err(serde::de::Error::duplicate_field("bloomFilterB64"));
                            }
                            bloom_filter_b64__ = Some(map_.next_value()?);
                        }
                        GeneratedField::CommunityIds => {
                            if community_ids__.is_some() {
                                return Err(serde::de::Error::duplicate_field("communityIds"));
                            }
                            community_ids__ = Some(map_.next_value()?);
                        }
                        GeneratedField::CreatedTs => {
                            if created_ts__.is_some() {
                                return Err(serde::de::Error::duplicate_field("createdTs"));
                            }
                            created_ts__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                    }
                }
                Ok(PcapIndexMeta {
                    tenant_id: tenant_id__.unwrap_or_default(),
                    probe_id: probe_id__.unwrap_or_default(),
                    file_key: file_key__.unwrap_or_default(),
                    ts_start: ts_start__.unwrap_or_default(),
                    ts_end: ts_end__.unwrap_or_default(),
                    byte_size: byte_size__.unwrap_or_default(),
                    zstd_level: zstd_level__.unwrap_or_default(),
                    sha256: sha256__.unwrap_or_default(),
                    community_id: community_id__.unwrap_or_default(),
                    flow_id: flow_id__.unwrap_or_default(),
                    offset_start: offset_start__.unwrap_or_default(),
                    offset_end: offset_end__.unwrap_or_default(),
                    bloom_filter_b64: bloom_filter_b64__.unwrap_or_default(),
                    community_ids: community_ids__.unwrap_or_default(),
                    created_ts: created_ts__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.PcapIndexMeta", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for ProbeConfig {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.config_version.is_empty() {
            len += 1;
        }
        if self.sample_rate != 0. {
            len += 1;
        }
        if !self.bpf_filter.is_empty() {
            len += 1;
        }
        if self.idle_timeout_sec != 0 {
            len += 1;
        }
        if self.active_timeout_sec != 0 {
            len += 1;
        }
        if self.batch_size != 0 {
            len += 1;
        }
        if !self.feature_set_version.is_empty() {
            len += 1;
        }
        if self.nic_config.is_some() {
            len += 1;
        }
        if self.ring_buffer_size != 0 {
            len += 1;
        }
        if self.batch_drain_timeout_ms != 0 {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.ProbeConfig", len)?;
        if !self.config_version.is_empty() {
            struct_ser.serialize_field("configVersion", &self.config_version)?;
        }
        if self.sample_rate != 0. {
            struct_ser.serialize_field("sampleRate", &self.sample_rate)?;
        }
        if !self.bpf_filter.is_empty() {
            struct_ser.serialize_field("bpfFilter", &self.bpf_filter)?;
        }
        if self.idle_timeout_sec != 0 {
            struct_ser.serialize_field("idleTimeoutSec", &self.idle_timeout_sec)?;
        }
        if self.active_timeout_sec != 0 {
            struct_ser.serialize_field("activeTimeoutSec", &self.active_timeout_sec)?;
        }
        if self.batch_size != 0 {
            struct_ser.serialize_field("batchSize", &self.batch_size)?;
        }
        if !self.feature_set_version.is_empty() {
            struct_ser.serialize_field("featureSetVersion", &self.feature_set_version)?;
        }
        if let Some(v) = self.nic_config.as_ref() {
            struct_ser.serialize_field("nicConfig", v)?;
        }
        if self.ring_buffer_size != 0 {
            struct_ser.serialize_field("ringBufferSize", &self.ring_buffer_size)?;
        }
        if self.batch_drain_timeout_ms != 0 {
            struct_ser.serialize_field("batchDrainTimeoutMs", &self.batch_drain_timeout_ms)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for ProbeConfig {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "config_version",
            "configVersion",
            "sample_rate",
            "sampleRate",
            "bpf_filter",
            "bpfFilter",
            "idle_timeout_sec",
            "idleTimeoutSec",
            "active_timeout_sec",
            "activeTimeoutSec",
            "batch_size",
            "batchSize",
            "feature_set_version",
            "featureSetVersion",
            "nic_config",
            "nicConfig",
            "ring_buffer_size",
            "ringBufferSize",
            "batch_drain_timeout_ms",
            "batchDrainTimeoutMs",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            ConfigVersion,
            SampleRate,
            BpfFilter,
            IdleTimeoutSec,
            ActiveTimeoutSec,
            BatchSize,
            FeatureSetVersion,
            NicConfig,
            RingBufferSize,
            BatchDrainTimeoutMs,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "configVersion" | "config_version" => Ok(GeneratedField::ConfigVersion),
                            "sampleRate" | "sample_rate" => Ok(GeneratedField::SampleRate),
                            "bpfFilter" | "bpf_filter" => Ok(GeneratedField::BpfFilter),
                            "idleTimeoutSec" | "idle_timeout_sec" => Ok(GeneratedField::IdleTimeoutSec),
                            "activeTimeoutSec" | "active_timeout_sec" => Ok(GeneratedField::ActiveTimeoutSec),
                            "batchSize" | "batch_size" => Ok(GeneratedField::BatchSize),
                            "featureSetVersion" | "feature_set_version" => Ok(GeneratedField::FeatureSetVersion),
                            "nicConfig" | "nic_config" => Ok(GeneratedField::NicConfig),
                            "ringBufferSize" | "ring_buffer_size" => Ok(GeneratedField::RingBufferSize),
                            "batchDrainTimeoutMs" | "batch_drain_timeout_ms" => Ok(GeneratedField::BatchDrainTimeoutMs),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = ProbeConfig;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.ProbeConfig")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<ProbeConfig, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut config_version__ = None;
                let mut sample_rate__ = None;
                let mut bpf_filter__ = None;
                let mut idle_timeout_sec__ = None;
                let mut active_timeout_sec__ = None;
                let mut batch_size__ = None;
                let mut feature_set_version__ = None;
                let mut nic_config__ = None;
                let mut ring_buffer_size__ = None;
                let mut batch_drain_timeout_ms__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::ConfigVersion => {
                            if config_version__.is_some() {
                                return Err(serde::de::Error::duplicate_field("configVersion"));
                            }
                            config_version__ = Some(map_.next_value()?);
                        }
                        GeneratedField::SampleRate => {
                            if sample_rate__.is_some() {
                                return Err(serde::de::Error::duplicate_field("sampleRate"));
                            }
                            sample_rate__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::BpfFilter => {
                            if bpf_filter__.is_some() {
                                return Err(serde::de::Error::duplicate_field("bpfFilter"));
                            }
                            bpf_filter__ = Some(map_.next_value()?);
                        }
                        GeneratedField::IdleTimeoutSec => {
                            if idle_timeout_sec__.is_some() {
                                return Err(serde::de::Error::duplicate_field("idleTimeoutSec"));
                            }
                            idle_timeout_sec__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::ActiveTimeoutSec => {
                            if active_timeout_sec__.is_some() {
                                return Err(serde::de::Error::duplicate_field("activeTimeoutSec"));
                            }
                            active_timeout_sec__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::BatchSize => {
                            if batch_size__.is_some() {
                                return Err(serde::de::Error::duplicate_field("batchSize"));
                            }
                            batch_size__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::FeatureSetVersion => {
                            if feature_set_version__.is_some() {
                                return Err(serde::de::Error::duplicate_field("featureSetVersion"));
                            }
                            feature_set_version__ = Some(map_.next_value()?);
                        }
                        GeneratedField::NicConfig => {
                            if nic_config__.is_some() {
                                return Err(serde::de::Error::duplicate_field("nicConfig"));
                            }
                            nic_config__ = map_.next_value()?;
                        }
                        GeneratedField::RingBufferSize => {
                            if ring_buffer_size__.is_some() {
                                return Err(serde::de::Error::duplicate_field("ringBufferSize"));
                            }
                            ring_buffer_size__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::BatchDrainTimeoutMs => {
                            if batch_drain_timeout_ms__.is_some() {
                                return Err(serde::de::Error::duplicate_field("batchDrainTimeoutMs"));
                            }
                            batch_drain_timeout_ms__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                    }
                }
                Ok(ProbeConfig {
                    config_version: config_version__.unwrap_or_default(),
                    sample_rate: sample_rate__.unwrap_or_default(),
                    bpf_filter: bpf_filter__.unwrap_or_default(),
                    idle_timeout_sec: idle_timeout_sec__.unwrap_or_default(),
                    active_timeout_sec: active_timeout_sec__.unwrap_or_default(),
                    batch_size: batch_size__.unwrap_or_default(),
                    feature_set_version: feature_set_version__.unwrap_or_default(),
                    nic_config: nic_config__,
                    ring_buffer_size: ring_buffer_size__.unwrap_or_default(),
                    batch_drain_timeout_ms: batch_drain_timeout_ms__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.ProbeConfig", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for ProbeStatus {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if self.cpu_usage != 0. {
            len += 1;
        }
        if self.memory_usage != 0. {
            len += 1;
        }
        if self.capture_pps != 0 {
            len += 1;
        }
        if self.upload_bps != 0 {
            len += 1;
        }
        if self.packets_captured != 0 {
            len += 1;
        }
        if self.packets_dropped != 0 {
            len += 1;
        }
        if self.uptime_seconds != 0 {
            len += 1;
        }
        if !self.interfaces.is_empty() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.ProbeStatus", len)?;
        if self.cpu_usage != 0. {
            struct_ser.serialize_field("cpuUsage", &self.cpu_usage)?;
        }
        if self.memory_usage != 0. {
            struct_ser.serialize_field("memoryUsage", &self.memory_usage)?;
        }
        if self.capture_pps != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("capturePps", ToString::to_string(&self.capture_pps).as_str())?;
        }
        if self.upload_bps != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("uploadBps", ToString::to_string(&self.upload_bps).as_str())?;
        }
        if self.packets_captured != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("packetsCaptured", ToString::to_string(&self.packets_captured).as_str())?;
        }
        if self.packets_dropped != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("packetsDropped", ToString::to_string(&self.packets_dropped).as_str())?;
        }
        if self.uptime_seconds != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("uptimeSeconds", ToString::to_string(&self.uptime_seconds).as_str())?;
        }
        if !self.interfaces.is_empty() {
            struct_ser.serialize_field("interfaces", &self.interfaces)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for ProbeStatus {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "cpu_usage",
            "cpuUsage",
            "memory_usage",
            "memoryUsage",
            "capture_pps",
            "capturePps",
            "upload_bps",
            "uploadBps",
            "packets_captured",
            "packetsCaptured",
            "packets_dropped",
            "packetsDropped",
            "uptime_seconds",
            "uptimeSeconds",
            "interfaces",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            CpuUsage,
            MemoryUsage,
            CapturePps,
            UploadBps,
            PacketsCaptured,
            PacketsDropped,
            UptimeSeconds,
            Interfaces,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "cpuUsage" | "cpu_usage" => Ok(GeneratedField::CpuUsage),
                            "memoryUsage" | "memory_usage" => Ok(GeneratedField::MemoryUsage),
                            "capturePps" | "capture_pps" => Ok(GeneratedField::CapturePps),
                            "uploadBps" | "upload_bps" => Ok(GeneratedField::UploadBps),
                            "packetsCaptured" | "packets_captured" => Ok(GeneratedField::PacketsCaptured),
                            "packetsDropped" | "packets_dropped" => Ok(GeneratedField::PacketsDropped),
                            "uptimeSeconds" | "uptime_seconds" => Ok(GeneratedField::UptimeSeconds),
                            "interfaces" => Ok(GeneratedField::Interfaces),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = ProbeStatus;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.ProbeStatus")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<ProbeStatus, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut cpu_usage__ = None;
                let mut memory_usage__ = None;
                let mut capture_pps__ = None;
                let mut upload_bps__ = None;
                let mut packets_captured__ = None;
                let mut packets_dropped__ = None;
                let mut uptime_seconds__ = None;
                let mut interfaces__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::CpuUsage => {
                            if cpu_usage__.is_some() {
                                return Err(serde::de::Error::duplicate_field("cpuUsage"));
                            }
                            cpu_usage__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::MemoryUsage => {
                            if memory_usage__.is_some() {
                                return Err(serde::de::Error::duplicate_field("memoryUsage"));
                            }
                            memory_usage__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::CapturePps => {
                            if capture_pps__.is_some() {
                                return Err(serde::de::Error::duplicate_field("capturePps"));
                            }
                            capture_pps__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::UploadBps => {
                            if upload_bps__.is_some() {
                                return Err(serde::de::Error::duplicate_field("uploadBps"));
                            }
                            upload_bps__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::PacketsCaptured => {
                            if packets_captured__.is_some() {
                                return Err(serde::de::Error::duplicate_field("packetsCaptured"));
                            }
                            packets_captured__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::PacketsDropped => {
                            if packets_dropped__.is_some() {
                                return Err(serde::de::Error::duplicate_field("packetsDropped"));
                            }
                            packets_dropped__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::UptimeSeconds => {
                            if uptime_seconds__.is_some() {
                                return Err(serde::de::Error::duplicate_field("uptimeSeconds"));
                            }
                            uptime_seconds__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::Interfaces => {
                            if interfaces__.is_some() {
                                return Err(serde::de::Error::duplicate_field("interfaces"));
                            }
                            interfaces__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(ProbeStatus {
                    cpu_usage: cpu_usage__.unwrap_or_default(),
                    memory_usage: memory_usage__.unwrap_or_default(),
                    capture_pps: capture_pps__.unwrap_or_default(),
                    upload_bps: upload_bps__.unwrap_or_default(),
                    packets_captured: packets_captured__.unwrap_or_default(),
                    packets_dropped: packets_dropped__.unwrap_or_default(),
                    uptime_seconds: uptime_seconds__.unwrap_or_default(),
                    interfaces: interfaces__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.ProbeStatus", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for RecordMacIpBindingRequest {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.bindings.is_empty() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.RecordMacIpBindingRequest", len)?;
        if !self.bindings.is_empty() {
            struct_ser.serialize_field("bindings", &self.bindings)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for RecordMacIpBindingRequest {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "bindings",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Bindings,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "bindings" => Ok(GeneratedField::Bindings),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = RecordMacIpBindingRequest;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.RecordMacIpBindingRequest")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<RecordMacIpBindingRequest, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut bindings__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Bindings => {
                            if bindings__.is_some() {
                                return Err(serde::de::Error::duplicate_field("bindings"));
                            }
                            bindings__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(RecordMacIpBindingRequest {
                    bindings: bindings__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.RecordMacIpBindingRequest", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for RecordMacIpBindingResponse {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if self.accepted != 0 {
            len += 1;
        }
        if self.rejected != 0 {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.RecordMacIpBindingResponse", len)?;
        if self.accepted != 0 {
            struct_ser.serialize_field("accepted", &self.accepted)?;
        }
        if self.rejected != 0 {
            struct_ser.serialize_field("rejected", &self.rejected)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for RecordMacIpBindingResponse {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "accepted",
            "rejected",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Accepted,
            Rejected,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "accepted" => Ok(GeneratedField::Accepted),
                            "rejected" => Ok(GeneratedField::Rejected),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = RecordMacIpBindingResponse;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.RecordMacIpBindingResponse")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<RecordMacIpBindingResponse, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut accepted__ = None;
                let mut rejected__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Accepted => {
                            if accepted__.is_some() {
                                return Err(serde::de::Error::duplicate_field("accepted"));
                            }
                            accepted__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::Rejected => {
                            if rejected__.is_some() {
                                return Err(serde::de::Error::duplicate_field("rejected"));
                            }
                            rejected__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                    }
                }
                Ok(RecordMacIpBindingResponse {
                    accepted: accepted__.unwrap_or_default(),
                    rejected: rejected__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.RecordMacIpBindingResponse", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for RegisterProbeRequest {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.tenant_id.is_empty() {
            len += 1;
        }
        if !self.probe_id.is_empty() {
            len += 1;
        }
        if self.hardware.is_some() {
            len += 1;
        }
        if !self.software_version.is_empty() {
            len += 1;
        }
        if !self.build_commit.is_empty() {
            len += 1;
        }
        if self.build_timestamp != 0 {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.RegisterProbeRequest", len)?;
        if !self.tenant_id.is_empty() {
            struct_ser.serialize_field("tenantId", &self.tenant_id)?;
        }
        if !self.probe_id.is_empty() {
            struct_ser.serialize_field("probeId", &self.probe_id)?;
        }
        if let Some(v) = self.hardware.as_ref() {
            struct_ser.serialize_field("hardware", v)?;
        }
        if !self.software_version.is_empty() {
            struct_ser.serialize_field("softwareVersion", &self.software_version)?;
        }
        if !self.build_commit.is_empty() {
            struct_ser.serialize_field("buildCommit", &self.build_commit)?;
        }
        if self.build_timestamp != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("buildTimestamp", ToString::to_string(&self.build_timestamp).as_str())?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for RegisterProbeRequest {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "tenant_id",
            "tenantId",
            "probe_id",
            "probeId",
            "hardware",
            "software_version",
            "softwareVersion",
            "build_commit",
            "buildCommit",
            "build_timestamp",
            "buildTimestamp",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            TenantId,
            ProbeId,
            Hardware,
            SoftwareVersion,
            BuildCommit,
            BuildTimestamp,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "tenantId" | "tenant_id" => Ok(GeneratedField::TenantId),
                            "probeId" | "probe_id" => Ok(GeneratedField::ProbeId),
                            "hardware" => Ok(GeneratedField::Hardware),
                            "softwareVersion" | "software_version" => Ok(GeneratedField::SoftwareVersion),
                            "buildCommit" | "build_commit" => Ok(GeneratedField::BuildCommit),
                            "buildTimestamp" | "build_timestamp" => Ok(GeneratedField::BuildTimestamp),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = RegisterProbeRequest;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.RegisterProbeRequest")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<RegisterProbeRequest, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut tenant_id__ = None;
                let mut probe_id__ = None;
                let mut hardware__ = None;
                let mut software_version__ = None;
                let mut build_commit__ = None;
                let mut build_timestamp__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::TenantId => {
                            if tenant_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("tenantId"));
                            }
                            tenant_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::ProbeId => {
                            if probe_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("probeId"));
                            }
                            probe_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Hardware => {
                            if hardware__.is_some() {
                                return Err(serde::de::Error::duplicate_field("hardware"));
                            }
                            hardware__ = map_.next_value()?;
                        }
                        GeneratedField::SoftwareVersion => {
                            if software_version__.is_some() {
                                return Err(serde::de::Error::duplicate_field("softwareVersion"));
                            }
                            software_version__ = Some(map_.next_value()?);
                        }
                        GeneratedField::BuildCommit => {
                            if build_commit__.is_some() {
                                return Err(serde::de::Error::duplicate_field("buildCommit"));
                            }
                            build_commit__ = Some(map_.next_value()?);
                        }
                        GeneratedField::BuildTimestamp => {
                            if build_timestamp__.is_some() {
                                return Err(serde::de::Error::duplicate_field("buildTimestamp"));
                            }
                            build_timestamp__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                    }
                }
                Ok(RegisterProbeRequest {
                    tenant_id: tenant_id__.unwrap_or_default(),
                    probe_id: probe_id__.unwrap_or_default(),
                    hardware: hardware__,
                    software_version: software_version__.unwrap_or_default(),
                    build_commit: build_commit__.unwrap_or_default(),
                    build_timestamp: build_timestamp__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.RegisterProbeRequest", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for RegisterProbeResponse {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if self.success {
            len += 1;
        }
        if !self.message.is_empty() {
            len += 1;
        }
        if self.initial_config.is_some() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.RegisterProbeResponse", len)?;
        if self.success {
            struct_ser.serialize_field("success", &self.success)?;
        }
        if !self.message.is_empty() {
            struct_ser.serialize_field("message", &self.message)?;
        }
        if let Some(v) = self.initial_config.as_ref() {
            struct_ser.serialize_field("initialConfig", v)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for RegisterProbeResponse {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "success",
            "message",
            "initial_config",
            "initialConfig",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Success,
            Message,
            InitialConfig,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "success" => Ok(GeneratedField::Success),
                            "message" => Ok(GeneratedField::Message),
                            "initialConfig" | "initial_config" => Ok(GeneratedField::InitialConfig),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = RegisterProbeResponse;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.RegisterProbeResponse")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<RegisterProbeResponse, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut success__ = None;
                let mut message__ = None;
                let mut initial_config__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Success => {
                            if success__.is_some() {
                                return Err(serde::de::Error::duplicate_field("success"));
                            }
                            success__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Message => {
                            if message__.is_some() {
                                return Err(serde::de::Error::duplicate_field("message"));
                            }
                            message__ = Some(map_.next_value()?);
                        }
                        GeneratedField::InitialConfig => {
                            if initial_config__.is_some() {
                                return Err(serde::de::Error::duplicate_field("initialConfig"));
                            }
                            initial_config__ = map_.next_value()?;
                        }
                    }
                }
                Ok(RegisterProbeResponse {
                    success: success__.unwrap_or_default(),
                    message: message__.unwrap_or_default(),
                    initial_config: initial_config__,
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.RegisterProbeResponse", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for SessionBatch {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.sessions.is_empty() {
            len += 1;
        }
        if !self.batch_id.is_empty() {
            len += 1;
        }
        if !self.tenant_id.is_empty() {
            len += 1;
        }
        if !self.probe_id.is_empty() {
            len += 1;
        }
        if !self.run_id.is_empty() {
            len += 1;
        }
        if self.batch_size != 0 {
            len += 1;
        }
        if !self.compression.is_empty() {
            len += 1;
        }
        if self.created_at != 0 {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.SessionBatch", len)?;
        if !self.sessions.is_empty() {
            struct_ser.serialize_field("sessions", &self.sessions)?;
        }
        if !self.batch_id.is_empty() {
            struct_ser.serialize_field("batchId", &self.batch_id)?;
        }
        if !self.tenant_id.is_empty() {
            struct_ser.serialize_field("tenantId", &self.tenant_id)?;
        }
        if !self.probe_id.is_empty() {
            struct_ser.serialize_field("probeId", &self.probe_id)?;
        }
        if !self.run_id.is_empty() {
            struct_ser.serialize_field("runId", &self.run_id)?;
        }
        if self.batch_size != 0 {
            struct_ser.serialize_field("batchSize", &self.batch_size)?;
        }
        if !self.compression.is_empty() {
            struct_ser.serialize_field("compression", &self.compression)?;
        }
        if self.created_at != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("createdAt", ToString::to_string(&self.created_at).as_str())?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for SessionBatch {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "sessions",
            "batch_id",
            "batchId",
            "tenant_id",
            "tenantId",
            "probe_id",
            "probeId",
            "run_id",
            "runId",
            "batch_size",
            "batchSize",
            "compression",
            "created_at",
            "createdAt",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Sessions,
            BatchId,
            TenantId,
            ProbeId,
            RunId,
            BatchSize,
            Compression,
            CreatedAt,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "sessions" => Ok(GeneratedField::Sessions),
                            "batchId" | "batch_id" => Ok(GeneratedField::BatchId),
                            "tenantId" | "tenant_id" => Ok(GeneratedField::TenantId),
                            "probeId" | "probe_id" => Ok(GeneratedField::ProbeId),
                            "runId" | "run_id" => Ok(GeneratedField::RunId),
                            "batchSize" | "batch_size" => Ok(GeneratedField::BatchSize),
                            "compression" => Ok(GeneratedField::Compression),
                            "createdAt" | "created_at" => Ok(GeneratedField::CreatedAt),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = SessionBatch;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.SessionBatch")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<SessionBatch, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut sessions__ = None;
                let mut batch_id__ = None;
                let mut tenant_id__ = None;
                let mut probe_id__ = None;
                let mut run_id__ = None;
                let mut batch_size__ = None;
                let mut compression__ = None;
                let mut created_at__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Sessions => {
                            if sessions__.is_some() {
                                return Err(serde::de::Error::duplicate_field("sessions"));
                            }
                            sessions__ = Some(map_.next_value()?);
                        }
                        GeneratedField::BatchId => {
                            if batch_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("batchId"));
                            }
                            batch_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::TenantId => {
                            if tenant_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("tenantId"));
                            }
                            tenant_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::ProbeId => {
                            if probe_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("probeId"));
                            }
                            probe_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::RunId => {
                            if run_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("runId"));
                            }
                            run_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::BatchSize => {
                            if batch_size__.is_some() {
                                return Err(serde::de::Error::duplicate_field("batchSize"));
                            }
                            batch_size__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::Compression => {
                            if compression__.is_some() {
                                return Err(serde::de::Error::duplicate_field("compression"));
                            }
                            compression__ = Some(map_.next_value()?);
                        }
                        GeneratedField::CreatedAt => {
                            if created_at__.is_some() {
                                return Err(serde::de::Error::duplicate_field("createdAt"));
                            }
                            created_at__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                    }
                }
                Ok(SessionBatch {
                    sessions: sessions__.unwrap_or_default(),
                    batch_id: batch_id__.unwrap_or_default(),
                    tenant_id: tenant_id__.unwrap_or_default(),
                    probe_id: probe_id__.unwrap_or_default(),
                    run_id: run_id__.unwrap_or_default(),
                    batch_size: batch_size__.unwrap_or_default(),
                    compression: compression__.unwrap_or_default(),
                    created_at: created_at__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.SessionBatch", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for SessionEvent {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if self.header.is_some() {
            len += 1;
        }
        if !self.session_id.is_empty() {
            len += 1;
        }
        if !self.community_id.is_empty() {
            len += 1;
        }
        if self.tuple.is_some() {
            len += 1;
        }
        if self.ts_start != 0 {
            len += 1;
        }
        if self.ts_end != 0 {
            len += 1;
        }
        if self.duration_ms != 0 {
            len += 1;
        }
        if self.protocol != 0 {
            len += 1;
        }
        if !self.client_ip.is_empty() {
            len += 1;
        }
        if !self.server_ip.is_empty() {
            len += 1;
        }
        if self.client_port != 0 {
            len += 1;
        }
        if self.server_port != 0 {
            len += 1;
        }
        if self.packets_total != 0 {
            len += 1;
        }
        if self.bytes_total != 0 {
            len += 1;
        }
        if self.bytes_fwd != 0 {
            len += 1;
        }
        if self.bytes_bwd != 0 {
            len += 1;
        }
        if self.up_down_ratio != 0. {
            len += 1;
        }
        if self.num_pkts != 0 {
            len += 1;
        }
        if self.avg_payload != 0. {
            len += 1;
        }
        if self.min_payload != 0 {
            len += 1;
        }
        if self.max_payload != 0 {
            len += 1;
        }
        if self.std_payload != 0. {
            len += 1;
        }
        if self.mean_iat_ms != 0. {
            len += 1;
        }
        if self.min_iat_ms != 0. {
            len += 1;
        }
        if self.max_iat_ms != 0. {
            len += 1;
        }
        if self.std_iat_ms != 0. {
            len += 1;
        }
        if self.flags_syn != 0 {
            len += 1;
        }
        if self.flags_ack != 0 {
            len += 1;
        }
        if self.flags_fin != 0 {
            len += 1;
        }
        if self.flags_psh != 0 {
            len += 1;
        }
        if self.flags_rst != 0 {
            len += 1;
        }
        if self.dns_pkt_cnt != 0 {
            len += 1;
        }
        if self.tcp_pkt_cnt != 0 {
            len += 1;
        }
        if self.udp_pkt_cnt != 0 {
            len += 1;
        }
        if self.icmp_pkt_cnt != 0 {
            len += 1;
        }
        if self.has_syn {
            len += 1;
        }
        if self.has_fin {
            len += 1;
        }
        if self.has_rst {
            len += 1;
        }
        if self.is_established {
            len += 1;
        }
        if self.evidence_count != 0 {
            len += 1;
        }
        if !self.flow_ids.is_empty() {
            len += 1;
        }
        if !self.end_reason.is_empty() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.SessionEvent", len)?;
        if let Some(v) = self.header.as_ref() {
            struct_ser.serialize_field("header", v)?;
        }
        if !self.session_id.is_empty() {
            struct_ser.serialize_field("sessionId", &self.session_id)?;
        }
        if !self.community_id.is_empty() {
            struct_ser.serialize_field("communityId", &self.community_id)?;
        }
        if let Some(v) = self.tuple.as_ref() {
            struct_ser.serialize_field("tuple", v)?;
        }
        if self.ts_start != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("tsStart", ToString::to_string(&self.ts_start).as_str())?;
        }
        if self.ts_end != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("tsEnd", ToString::to_string(&self.ts_end).as_str())?;
        }
        if self.duration_ms != 0 {
            struct_ser.serialize_field("durationMs", &self.duration_ms)?;
        }
        if self.protocol != 0 {
            struct_ser.serialize_field("protocol", &self.protocol)?;
        }
        if !self.client_ip.is_empty() {
            struct_ser.serialize_field("clientIp", &self.client_ip)?;
        }
        if !self.server_ip.is_empty() {
            struct_ser.serialize_field("serverIp", &self.server_ip)?;
        }
        if self.client_port != 0 {
            struct_ser.serialize_field("clientPort", &self.client_port)?;
        }
        if self.server_port != 0 {
            struct_ser.serialize_field("serverPort", &self.server_port)?;
        }
        if self.packets_total != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("packetsTotal", ToString::to_string(&self.packets_total).as_str())?;
        }
        if self.bytes_total != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("bytesTotal", ToString::to_string(&self.bytes_total).as_str())?;
        }
        if self.bytes_fwd != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("bytesFwd", ToString::to_string(&self.bytes_fwd).as_str())?;
        }
        if self.bytes_bwd != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("bytesBwd", ToString::to_string(&self.bytes_bwd).as_str())?;
        }
        if self.up_down_ratio != 0. {
            struct_ser.serialize_field("upDownRatio", &self.up_down_ratio)?;
        }
        if self.num_pkts != 0 {
            struct_ser.serialize_field("numPkts", &self.num_pkts)?;
        }
        if self.avg_payload != 0. {
            struct_ser.serialize_field("avgPayload", &self.avg_payload)?;
        }
        if self.min_payload != 0 {
            struct_ser.serialize_field("minPayload", &self.min_payload)?;
        }
        if self.max_payload != 0 {
            struct_ser.serialize_field("maxPayload", &self.max_payload)?;
        }
        if self.std_payload != 0. {
            struct_ser.serialize_field("stdPayload", &self.std_payload)?;
        }
        if self.mean_iat_ms != 0. {
            struct_ser.serialize_field("meanIatMs", &self.mean_iat_ms)?;
        }
        if self.min_iat_ms != 0. {
            struct_ser.serialize_field("minIatMs", &self.min_iat_ms)?;
        }
        if self.max_iat_ms != 0. {
            struct_ser.serialize_field("maxIatMs", &self.max_iat_ms)?;
        }
        if self.std_iat_ms != 0. {
            struct_ser.serialize_field("stdIatMs", &self.std_iat_ms)?;
        }
        if self.flags_syn != 0 {
            struct_ser.serialize_field("flagsSyn", &self.flags_syn)?;
        }
        if self.flags_ack != 0 {
            struct_ser.serialize_field("flagsAck", &self.flags_ack)?;
        }
        if self.flags_fin != 0 {
            struct_ser.serialize_field("flagsFin", &self.flags_fin)?;
        }
        if self.flags_psh != 0 {
            struct_ser.serialize_field("flagsPsh", &self.flags_psh)?;
        }
        if self.flags_rst != 0 {
            struct_ser.serialize_field("flagsRst", &self.flags_rst)?;
        }
        if self.dns_pkt_cnt != 0 {
            struct_ser.serialize_field("dnsPktCnt", &self.dns_pkt_cnt)?;
        }
        if self.tcp_pkt_cnt != 0 {
            struct_ser.serialize_field("tcpPktCnt", &self.tcp_pkt_cnt)?;
        }
        if self.udp_pkt_cnt != 0 {
            struct_ser.serialize_field("udpPktCnt", &self.udp_pkt_cnt)?;
        }
        if self.icmp_pkt_cnt != 0 {
            struct_ser.serialize_field("icmpPktCnt", &self.icmp_pkt_cnt)?;
        }
        if self.has_syn {
            struct_ser.serialize_field("hasSyn", &self.has_syn)?;
        }
        if self.has_fin {
            struct_ser.serialize_field("hasFin", &self.has_fin)?;
        }
        if self.has_rst {
            struct_ser.serialize_field("hasRst", &self.has_rst)?;
        }
        if self.is_established {
            struct_ser.serialize_field("isEstablished", &self.is_established)?;
        }
        if self.evidence_count != 0 {
            struct_ser.serialize_field("evidenceCount", &self.evidence_count)?;
        }
        if !self.flow_ids.is_empty() {
            struct_ser.serialize_field("flowIds", &self.flow_ids)?;
        }
        if !self.end_reason.is_empty() {
            struct_ser.serialize_field("endReason", &self.end_reason)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for SessionEvent {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "header",
            "session_id",
            "sessionId",
            "community_id",
            "communityId",
            "tuple",
            "ts_start",
            "tsStart",
            "ts_end",
            "tsEnd",
            "duration_ms",
            "durationMs",
            "protocol",
            "client_ip",
            "clientIp",
            "server_ip",
            "serverIp",
            "client_port",
            "clientPort",
            "server_port",
            "serverPort",
            "packets_total",
            "packetsTotal",
            "bytes_total",
            "bytesTotal",
            "bytes_fwd",
            "bytesFwd",
            "bytes_bwd",
            "bytesBwd",
            "up_down_ratio",
            "upDownRatio",
            "num_pkts",
            "numPkts",
            "avg_payload",
            "avgPayload",
            "min_payload",
            "minPayload",
            "max_payload",
            "maxPayload",
            "std_payload",
            "stdPayload",
            "mean_iat_ms",
            "meanIatMs",
            "min_iat_ms",
            "minIatMs",
            "max_iat_ms",
            "maxIatMs",
            "std_iat_ms",
            "stdIatMs",
            "flags_syn",
            "flagsSyn",
            "flags_ack",
            "flagsAck",
            "flags_fin",
            "flagsFin",
            "flags_psh",
            "flagsPsh",
            "flags_rst",
            "flagsRst",
            "dns_pkt_cnt",
            "dnsPktCnt",
            "tcp_pkt_cnt",
            "tcpPktCnt",
            "udp_pkt_cnt",
            "udpPktCnt",
            "icmp_pkt_cnt",
            "icmpPktCnt",
            "has_syn",
            "hasSyn",
            "has_fin",
            "hasFin",
            "has_rst",
            "hasRst",
            "is_established",
            "isEstablished",
            "evidence_count",
            "evidenceCount",
            "flow_ids",
            "flowIds",
            "end_reason",
            "endReason",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Header,
            SessionId,
            CommunityId,
            Tuple,
            TsStart,
            TsEnd,
            DurationMs,
            Protocol,
            ClientIp,
            ServerIp,
            ClientPort,
            ServerPort,
            PacketsTotal,
            BytesTotal,
            BytesFwd,
            BytesBwd,
            UpDownRatio,
            NumPkts,
            AvgPayload,
            MinPayload,
            MaxPayload,
            StdPayload,
            MeanIatMs,
            MinIatMs,
            MaxIatMs,
            StdIatMs,
            FlagsSyn,
            FlagsAck,
            FlagsFin,
            FlagsPsh,
            FlagsRst,
            DnsPktCnt,
            TcpPktCnt,
            UdpPktCnt,
            IcmpPktCnt,
            HasSyn,
            HasFin,
            HasRst,
            IsEstablished,
            EvidenceCount,
            FlowIds,
            EndReason,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "header" => Ok(GeneratedField::Header),
                            "sessionId" | "session_id" => Ok(GeneratedField::SessionId),
                            "communityId" | "community_id" => Ok(GeneratedField::CommunityId),
                            "tuple" => Ok(GeneratedField::Tuple),
                            "tsStart" | "ts_start" => Ok(GeneratedField::TsStart),
                            "tsEnd" | "ts_end" => Ok(GeneratedField::TsEnd),
                            "durationMs" | "duration_ms" => Ok(GeneratedField::DurationMs),
                            "protocol" => Ok(GeneratedField::Protocol),
                            "clientIp" | "client_ip" => Ok(GeneratedField::ClientIp),
                            "serverIp" | "server_ip" => Ok(GeneratedField::ServerIp),
                            "clientPort" | "client_port" => Ok(GeneratedField::ClientPort),
                            "serverPort" | "server_port" => Ok(GeneratedField::ServerPort),
                            "packetsTotal" | "packets_total" => Ok(GeneratedField::PacketsTotal),
                            "bytesTotal" | "bytes_total" => Ok(GeneratedField::BytesTotal),
                            "bytesFwd" | "bytes_fwd" => Ok(GeneratedField::BytesFwd),
                            "bytesBwd" | "bytes_bwd" => Ok(GeneratedField::BytesBwd),
                            "upDownRatio" | "up_down_ratio" => Ok(GeneratedField::UpDownRatio),
                            "numPkts" | "num_pkts" => Ok(GeneratedField::NumPkts),
                            "avgPayload" | "avg_payload" => Ok(GeneratedField::AvgPayload),
                            "minPayload" | "min_payload" => Ok(GeneratedField::MinPayload),
                            "maxPayload" | "max_payload" => Ok(GeneratedField::MaxPayload),
                            "stdPayload" | "std_payload" => Ok(GeneratedField::StdPayload),
                            "meanIatMs" | "mean_iat_ms" => Ok(GeneratedField::MeanIatMs),
                            "minIatMs" | "min_iat_ms" => Ok(GeneratedField::MinIatMs),
                            "maxIatMs" | "max_iat_ms" => Ok(GeneratedField::MaxIatMs),
                            "stdIatMs" | "std_iat_ms" => Ok(GeneratedField::StdIatMs),
                            "flagsSyn" | "flags_syn" => Ok(GeneratedField::FlagsSyn),
                            "flagsAck" | "flags_ack" => Ok(GeneratedField::FlagsAck),
                            "flagsFin" | "flags_fin" => Ok(GeneratedField::FlagsFin),
                            "flagsPsh" | "flags_psh" => Ok(GeneratedField::FlagsPsh),
                            "flagsRst" | "flags_rst" => Ok(GeneratedField::FlagsRst),
                            "dnsPktCnt" | "dns_pkt_cnt" => Ok(GeneratedField::DnsPktCnt),
                            "tcpPktCnt" | "tcp_pkt_cnt" => Ok(GeneratedField::TcpPktCnt),
                            "udpPktCnt" | "udp_pkt_cnt" => Ok(GeneratedField::UdpPktCnt),
                            "icmpPktCnt" | "icmp_pkt_cnt" => Ok(GeneratedField::IcmpPktCnt),
                            "hasSyn" | "has_syn" => Ok(GeneratedField::HasSyn),
                            "hasFin" | "has_fin" => Ok(GeneratedField::HasFin),
                            "hasRst" | "has_rst" => Ok(GeneratedField::HasRst),
                            "isEstablished" | "is_established" => Ok(GeneratedField::IsEstablished),
                            "evidenceCount" | "evidence_count" => Ok(GeneratedField::EvidenceCount),
                            "flowIds" | "flow_ids" => Ok(GeneratedField::FlowIds),
                            "endReason" | "end_reason" => Ok(GeneratedField::EndReason),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = SessionEvent;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.SessionEvent")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<SessionEvent, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut header__ = None;
                let mut session_id__ = None;
                let mut community_id__ = None;
                let mut tuple__ = None;
                let mut ts_start__ = None;
                let mut ts_end__ = None;
                let mut duration_ms__ = None;
                let mut protocol__ = None;
                let mut client_ip__ = None;
                let mut server_ip__ = None;
                let mut client_port__ = None;
                let mut server_port__ = None;
                let mut packets_total__ = None;
                let mut bytes_total__ = None;
                let mut bytes_fwd__ = None;
                let mut bytes_bwd__ = None;
                let mut up_down_ratio__ = None;
                let mut num_pkts__ = None;
                let mut avg_payload__ = None;
                let mut min_payload__ = None;
                let mut max_payload__ = None;
                let mut std_payload__ = None;
                let mut mean_iat_ms__ = None;
                let mut min_iat_ms__ = None;
                let mut max_iat_ms__ = None;
                let mut std_iat_ms__ = None;
                let mut flags_syn__ = None;
                let mut flags_ack__ = None;
                let mut flags_fin__ = None;
                let mut flags_psh__ = None;
                let mut flags_rst__ = None;
                let mut dns_pkt_cnt__ = None;
                let mut tcp_pkt_cnt__ = None;
                let mut udp_pkt_cnt__ = None;
                let mut icmp_pkt_cnt__ = None;
                let mut has_syn__ = None;
                let mut has_fin__ = None;
                let mut has_rst__ = None;
                let mut is_established__ = None;
                let mut evidence_count__ = None;
                let mut flow_ids__ = None;
                let mut end_reason__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Header => {
                            if header__.is_some() {
                                return Err(serde::de::Error::duplicate_field("header"));
                            }
                            header__ = map_.next_value()?;
                        }
                        GeneratedField::SessionId => {
                            if session_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("sessionId"));
                            }
                            session_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::CommunityId => {
                            if community_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("communityId"));
                            }
                            community_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Tuple => {
                            if tuple__.is_some() {
                                return Err(serde::de::Error::duplicate_field("tuple"));
                            }
                            tuple__ = map_.next_value()?;
                        }
                        GeneratedField::TsStart => {
                            if ts_start__.is_some() {
                                return Err(serde::de::Error::duplicate_field("tsStart"));
                            }
                            ts_start__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::TsEnd => {
                            if ts_end__.is_some() {
                                return Err(serde::de::Error::duplicate_field("tsEnd"));
                            }
                            ts_end__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::DurationMs => {
                            if duration_ms__.is_some() {
                                return Err(serde::de::Error::duplicate_field("durationMs"));
                            }
                            duration_ms__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::Protocol => {
                            if protocol__.is_some() {
                                return Err(serde::de::Error::duplicate_field("protocol"));
                            }
                            protocol__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::ClientIp => {
                            if client_ip__.is_some() {
                                return Err(serde::de::Error::duplicate_field("clientIp"));
                            }
                            client_ip__ = Some(map_.next_value()?);
                        }
                        GeneratedField::ServerIp => {
                            if server_ip__.is_some() {
                                return Err(serde::de::Error::duplicate_field("serverIp"));
                            }
                            server_ip__ = Some(map_.next_value()?);
                        }
                        GeneratedField::ClientPort => {
                            if client_port__.is_some() {
                                return Err(serde::de::Error::duplicate_field("clientPort"));
                            }
                            client_port__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::ServerPort => {
                            if server_port__.is_some() {
                                return Err(serde::de::Error::duplicate_field("serverPort"));
                            }
                            server_port__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::PacketsTotal => {
                            if packets_total__.is_some() {
                                return Err(serde::de::Error::duplicate_field("packetsTotal"));
                            }
                            packets_total__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::BytesTotal => {
                            if bytes_total__.is_some() {
                                return Err(serde::de::Error::duplicate_field("bytesTotal"));
                            }
                            bytes_total__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::BytesFwd => {
                            if bytes_fwd__.is_some() {
                                return Err(serde::de::Error::duplicate_field("bytesFwd"));
                            }
                            bytes_fwd__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::BytesBwd => {
                            if bytes_bwd__.is_some() {
                                return Err(serde::de::Error::duplicate_field("bytesBwd"));
                            }
                            bytes_bwd__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::UpDownRatio => {
                            if up_down_ratio__.is_some() {
                                return Err(serde::de::Error::duplicate_field("upDownRatio"));
                            }
                            up_down_ratio__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::NumPkts => {
                            if num_pkts__.is_some() {
                                return Err(serde::de::Error::duplicate_field("numPkts"));
                            }
                            num_pkts__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::AvgPayload => {
                            if avg_payload__.is_some() {
                                return Err(serde::de::Error::duplicate_field("avgPayload"));
                            }
                            avg_payload__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::MinPayload => {
                            if min_payload__.is_some() {
                                return Err(serde::de::Error::duplicate_field("minPayload"));
                            }
                            min_payload__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::MaxPayload => {
                            if max_payload__.is_some() {
                                return Err(serde::de::Error::duplicate_field("maxPayload"));
                            }
                            max_payload__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::StdPayload => {
                            if std_payload__.is_some() {
                                return Err(serde::de::Error::duplicate_field("stdPayload"));
                            }
                            std_payload__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::MeanIatMs => {
                            if mean_iat_ms__.is_some() {
                                return Err(serde::de::Error::duplicate_field("meanIatMs"));
                            }
                            mean_iat_ms__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::MinIatMs => {
                            if min_iat_ms__.is_some() {
                                return Err(serde::de::Error::duplicate_field("minIatMs"));
                            }
                            min_iat_ms__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::MaxIatMs => {
                            if max_iat_ms__.is_some() {
                                return Err(serde::de::Error::duplicate_field("maxIatMs"));
                            }
                            max_iat_ms__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::StdIatMs => {
                            if std_iat_ms__.is_some() {
                                return Err(serde::de::Error::duplicate_field("stdIatMs"));
                            }
                            std_iat_ms__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::FlagsSyn => {
                            if flags_syn__.is_some() {
                                return Err(serde::de::Error::duplicate_field("flagsSyn"));
                            }
                            flags_syn__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::FlagsAck => {
                            if flags_ack__.is_some() {
                                return Err(serde::de::Error::duplicate_field("flagsAck"));
                            }
                            flags_ack__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::FlagsFin => {
                            if flags_fin__.is_some() {
                                return Err(serde::de::Error::duplicate_field("flagsFin"));
                            }
                            flags_fin__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::FlagsPsh => {
                            if flags_psh__.is_some() {
                                return Err(serde::de::Error::duplicate_field("flagsPsh"));
                            }
                            flags_psh__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::FlagsRst => {
                            if flags_rst__.is_some() {
                                return Err(serde::de::Error::duplicate_field("flagsRst"));
                            }
                            flags_rst__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::DnsPktCnt => {
                            if dns_pkt_cnt__.is_some() {
                                return Err(serde::de::Error::duplicate_field("dnsPktCnt"));
                            }
                            dns_pkt_cnt__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::TcpPktCnt => {
                            if tcp_pkt_cnt__.is_some() {
                                return Err(serde::de::Error::duplicate_field("tcpPktCnt"));
                            }
                            tcp_pkt_cnt__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::UdpPktCnt => {
                            if udp_pkt_cnt__.is_some() {
                                return Err(serde::de::Error::duplicate_field("udpPktCnt"));
                            }
                            udp_pkt_cnt__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::IcmpPktCnt => {
                            if icmp_pkt_cnt__.is_some() {
                                return Err(serde::de::Error::duplicate_field("icmpPktCnt"));
                            }
                            icmp_pkt_cnt__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::HasSyn => {
                            if has_syn__.is_some() {
                                return Err(serde::de::Error::duplicate_field("hasSyn"));
                            }
                            has_syn__ = Some(map_.next_value()?);
                        }
                        GeneratedField::HasFin => {
                            if has_fin__.is_some() {
                                return Err(serde::de::Error::duplicate_field("hasFin"));
                            }
                            has_fin__ = Some(map_.next_value()?);
                        }
                        GeneratedField::HasRst => {
                            if has_rst__.is_some() {
                                return Err(serde::de::Error::duplicate_field("hasRst"));
                            }
                            has_rst__ = Some(map_.next_value()?);
                        }
                        GeneratedField::IsEstablished => {
                            if is_established__.is_some() {
                                return Err(serde::de::Error::duplicate_field("isEstablished"));
                            }
                            is_established__ = Some(map_.next_value()?);
                        }
                        GeneratedField::EvidenceCount => {
                            if evidence_count__.is_some() {
                                return Err(serde::de::Error::duplicate_field("evidenceCount"));
                            }
                            evidence_count__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::FlowIds => {
                            if flow_ids__.is_some() {
                                return Err(serde::de::Error::duplicate_field("flowIds"));
                            }
                            flow_ids__ = Some(map_.next_value()?);
                        }
                        GeneratedField::EndReason => {
                            if end_reason__.is_some() {
                                return Err(serde::de::Error::duplicate_field("endReason"));
                            }
                            end_reason__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(SessionEvent {
                    header: header__,
                    session_id: session_id__.unwrap_or_default(),
                    community_id: community_id__.unwrap_or_default(),
                    tuple: tuple__,
                    ts_start: ts_start__.unwrap_or_default(),
                    ts_end: ts_end__.unwrap_or_default(),
                    duration_ms: duration_ms__.unwrap_or_default(),
                    protocol: protocol__.unwrap_or_default(),
                    client_ip: client_ip__.unwrap_or_default(),
                    server_ip: server_ip__.unwrap_or_default(),
                    client_port: client_port__.unwrap_or_default(),
                    server_port: server_port__.unwrap_or_default(),
                    packets_total: packets_total__.unwrap_or_default(),
                    bytes_total: bytes_total__.unwrap_or_default(),
                    bytes_fwd: bytes_fwd__.unwrap_or_default(),
                    bytes_bwd: bytes_bwd__.unwrap_or_default(),
                    up_down_ratio: up_down_ratio__.unwrap_or_default(),
                    num_pkts: num_pkts__.unwrap_or_default(),
                    avg_payload: avg_payload__.unwrap_or_default(),
                    min_payload: min_payload__.unwrap_or_default(),
                    max_payload: max_payload__.unwrap_or_default(),
                    std_payload: std_payload__.unwrap_or_default(),
                    mean_iat_ms: mean_iat_ms__.unwrap_or_default(),
                    min_iat_ms: min_iat_ms__.unwrap_or_default(),
                    max_iat_ms: max_iat_ms__.unwrap_or_default(),
                    std_iat_ms: std_iat_ms__.unwrap_or_default(),
                    flags_syn: flags_syn__.unwrap_or_default(),
                    flags_ack: flags_ack__.unwrap_or_default(),
                    flags_fin: flags_fin__.unwrap_or_default(),
                    flags_psh: flags_psh__.unwrap_or_default(),
                    flags_rst: flags_rst__.unwrap_or_default(),
                    dns_pkt_cnt: dns_pkt_cnt__.unwrap_or_default(),
                    tcp_pkt_cnt: tcp_pkt_cnt__.unwrap_or_default(),
                    udp_pkt_cnt: udp_pkt_cnt__.unwrap_or_default(),
                    icmp_pkt_cnt: icmp_pkt_cnt__.unwrap_or_default(),
                    has_syn: has_syn__.unwrap_or_default(),
                    has_fin: has_fin__.unwrap_or_default(),
                    has_rst: has_rst__.unwrap_or_default(),
                    is_established: is_established__.unwrap_or_default(),
                    evidence_count: evidence_count__.unwrap_or_default(),
                    flow_ids: flow_ids__.unwrap_or_default(),
                    end_reason: end_reason__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.SessionEvent", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for Severity {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        let variant = match self {
            Self::Unspecified => "SEVERITY_UNSPECIFIED",
            Self::Info => "SEVERITY_INFO",
            Self::Low => "SEVERITY_LOW",
            Self::Medium => "SEVERITY_MEDIUM",
            Self::High => "SEVERITY_HIGH",
            Self::Critical => "SEVERITY_CRITICAL",
        };
        serializer.serialize_str(variant)
    }
}
impl<'de> serde::Deserialize<'de> for Severity {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "SEVERITY_UNSPECIFIED",
            "SEVERITY_INFO",
            "SEVERITY_LOW",
            "SEVERITY_MEDIUM",
            "SEVERITY_HIGH",
            "SEVERITY_CRITICAL",
        ];

        struct GeneratedVisitor;

        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = Severity;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                write!(formatter, "expected one of: {:?}", &FIELDS)
            }

            fn visit_i64<E>(self, v: i64) -> std::result::Result<Self::Value, E>
            where
                E: serde::de::Error,
            {
                i32::try_from(v)
                    .ok()
                    .and_then(|x| x.try_into().ok())
                    .ok_or_else(|| {
                        serde::de::Error::invalid_value(serde::de::Unexpected::Signed(v), &self)
                    })
            }

            fn visit_u64<E>(self, v: u64) -> std::result::Result<Self::Value, E>
            where
                E: serde::de::Error,
            {
                i32::try_from(v)
                    .ok()
                    .and_then(|x| x.try_into().ok())
                    .ok_or_else(|| {
                        serde::de::Error::invalid_value(serde::de::Unexpected::Unsigned(v), &self)
                    })
            }

            fn visit_str<E>(self, value: &str) -> std::result::Result<Self::Value, E>
            where
                E: serde::de::Error,
            {
                match value {
                    "SEVERITY_UNSPECIFIED" => Ok(Severity::Unspecified),
                    "SEVERITY_INFO" => Ok(Severity::Info),
                    "SEVERITY_LOW" => Ok(Severity::Low),
                    "SEVERITY_MEDIUM" => Ok(Severity::Medium),
                    "SEVERITY_HIGH" => Ok(Severity::High),
                    "SEVERITY_CRITICAL" => Ok(Severity::Critical),
                    _ => Err(serde::de::Error::unknown_variant(value, FIELDS)),
                }
            }
        }
        deserializer.deserialize_any(GeneratedVisitor)
    }
}
impl serde::Serialize for StorageHealthEvent {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.storage_type.is_empty() {
            len += 1;
        }
        if !self.storage_name.is_empty() {
            len += 1;
        }
        if !self.status.is_empty() {
            len += 1;
        }
        if !self.error_message.is_empty() {
            len += 1;
        }
        if self.consecutive_failures != 0 {
            len += 1;
        }
        if self.ts != 0 {
            len += 1;
        }
        if self.ingest_ts != 0 {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.StorageHealthEvent", len)?;
        if !self.storage_type.is_empty() {
            struct_ser.serialize_field("storageType", &self.storage_type)?;
        }
        if !self.storage_name.is_empty() {
            struct_ser.serialize_field("storageName", &self.storage_name)?;
        }
        if !self.status.is_empty() {
            struct_ser.serialize_field("status", &self.status)?;
        }
        if !self.error_message.is_empty() {
            struct_ser.serialize_field("errorMessage", &self.error_message)?;
        }
        if self.consecutive_failures != 0 {
            struct_ser.serialize_field("consecutiveFailures", &self.consecutive_failures)?;
        }
        if self.ts != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("ts", ToString::to_string(&self.ts).as_str())?;
        }
        if self.ingest_ts != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("ingestTs", ToString::to_string(&self.ingest_ts).as_str())?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for StorageHealthEvent {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "storage_type",
            "storageType",
            "storage_name",
            "storageName",
            "status",
            "error_message",
            "errorMessage",
            "consecutive_failures",
            "consecutiveFailures",
            "ts",
            "ingest_ts",
            "ingestTs",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            StorageType,
            StorageName,
            Status,
            ErrorMessage,
            ConsecutiveFailures,
            Ts,
            IngestTs,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "storageType" | "storage_type" => Ok(GeneratedField::StorageType),
                            "storageName" | "storage_name" => Ok(GeneratedField::StorageName),
                            "status" => Ok(GeneratedField::Status),
                            "errorMessage" | "error_message" => Ok(GeneratedField::ErrorMessage),
                            "consecutiveFailures" | "consecutive_failures" => Ok(GeneratedField::ConsecutiveFailures),
                            "ts" => Ok(GeneratedField::Ts),
                            "ingestTs" | "ingest_ts" => Ok(GeneratedField::IngestTs),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = StorageHealthEvent;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.StorageHealthEvent")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<StorageHealthEvent, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut storage_type__ = None;
                let mut storage_name__ = None;
                let mut status__ = None;
                let mut error_message__ = None;
                let mut consecutive_failures__ = None;
                let mut ts__ = None;
                let mut ingest_ts__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::StorageType => {
                            if storage_type__.is_some() {
                                return Err(serde::de::Error::duplicate_field("storageType"));
                            }
                            storage_type__ = Some(map_.next_value()?);
                        }
                        GeneratedField::StorageName => {
                            if storage_name__.is_some() {
                                return Err(serde::de::Error::duplicate_field("storageName"));
                            }
                            storage_name__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Status => {
                            if status__.is_some() {
                                return Err(serde::de::Error::duplicate_field("status"));
                            }
                            status__ = Some(map_.next_value()?);
                        }
                        GeneratedField::ErrorMessage => {
                            if error_message__.is_some() {
                                return Err(serde::de::Error::duplicate_field("errorMessage"));
                            }
                            error_message__ = Some(map_.next_value()?);
                        }
                        GeneratedField::ConsecutiveFailures => {
                            if consecutive_failures__.is_some() {
                                return Err(serde::de::Error::duplicate_field("consecutiveFailures"));
                            }
                            consecutive_failures__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::Ts => {
                            if ts__.is_some() {
                                return Err(serde::de::Error::duplicate_field("ts"));
                            }
                            ts__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::IngestTs => {
                            if ingest_ts__.is_some() {
                                return Err(serde::de::Error::duplicate_field("ingestTs"));
                            }
                            ingest_ts__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                    }
                }
                Ok(StorageHealthEvent {
                    storage_type: storage_type__.unwrap_or_default(),
                    storage_name: storage_name__.unwrap_or_default(),
                    status: status__.unwrap_or_default(),
                    error_message: error_message__.unwrap_or_default(),
                    consecutive_failures: consecutive_failures__.unwrap_or_default(),
                    ts: ts__.unwrap_or_default(),
                    ingest_ts: ingest_ts__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.StorageHealthEvent", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for StreamFlowsRequest {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if self.event.is_some() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.StreamFlowsRequest", len)?;
        if let Some(v) = self.event.as_ref() {
            struct_ser.serialize_field("event", v)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for StreamFlowsRequest {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "event",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Event,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "event" => Ok(GeneratedField::Event),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = StreamFlowsRequest;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.StreamFlowsRequest")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<StreamFlowsRequest, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut event__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Event => {
                            if event__.is_some() {
                                return Err(serde::de::Error::duplicate_field("event"));
                            }
                            event__ = map_.next_value()?;
                        }
                    }
                }
                Ok(StreamFlowsRequest {
                    event: event__,
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.StreamFlowsRequest", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for StreamFlowsResponse {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.event_id.is_empty() {
            len += 1;
        }
        if self.accepted {
            len += 1;
        }
        if !self.error.is_empty() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.StreamFlowsResponse", len)?;
        if !self.event_id.is_empty() {
            struct_ser.serialize_field("eventId", &self.event_id)?;
        }
        if self.accepted {
            struct_ser.serialize_field("accepted", &self.accepted)?;
        }
        if !self.error.is_empty() {
            struct_ser.serialize_field("error", &self.error)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for StreamFlowsResponse {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "event_id",
            "eventId",
            "accepted",
            "error",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            EventId,
            Accepted,
            Error,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "eventId" | "event_id" => Ok(GeneratedField::EventId),
                            "accepted" => Ok(GeneratedField::Accepted),
                            "error" => Ok(GeneratedField::Error),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = StreamFlowsResponse;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.StreamFlowsResponse")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<StreamFlowsResponse, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut event_id__ = None;
                let mut accepted__ = None;
                let mut error__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::EventId => {
                            if event_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("eventId"));
                            }
                            event_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Accepted => {
                            if accepted__.is_some() {
                                return Err(serde::de::Error::duplicate_field("accepted"));
                            }
                            accepted__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Error => {
                            if error__.is_some() {
                                return Err(serde::de::Error::duplicate_field("error"));
                            }
                            error__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(StreamFlowsResponse {
                    event_id: event_id__.unwrap_or_default(),
                    accepted: accepted__.unwrap_or_default(),
                    error: error__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.StreamFlowsResponse", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for TaskStatus {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        let variant = match self {
            Self::Unspecified => "TASK_STATUS_UNSPECIFIED",
            Self::Queued => "TASK_STATUS_QUEUED",
            Self::Running => "TASK_STATUS_RUNNING",
            Self::Succeeded => "TASK_STATUS_SUCCEEDED",
            Self::Failed => "TASK_STATUS_FAILED",
            Self::Canceled => "TASK_STATUS_CANCELED",
        };
        serializer.serialize_str(variant)
    }
}
impl<'de> serde::Deserialize<'de> for TaskStatus {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "TASK_STATUS_UNSPECIFIED",
            "TASK_STATUS_QUEUED",
            "TASK_STATUS_RUNNING",
            "TASK_STATUS_SUCCEEDED",
            "TASK_STATUS_FAILED",
            "TASK_STATUS_CANCELED",
        ];

        struct GeneratedVisitor;

        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = TaskStatus;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                write!(formatter, "expected one of: {:?}", &FIELDS)
            }

            fn visit_i64<E>(self, v: i64) -> std::result::Result<Self::Value, E>
            where
                E: serde::de::Error,
            {
                i32::try_from(v)
                    .ok()
                    .and_then(|x| x.try_into().ok())
                    .ok_or_else(|| {
                        serde::de::Error::invalid_value(serde::de::Unexpected::Signed(v), &self)
                    })
            }

            fn visit_u64<E>(self, v: u64) -> std::result::Result<Self::Value, E>
            where
                E: serde::de::Error,
            {
                i32::try_from(v)
                    .ok()
                    .and_then(|x| x.try_into().ok())
                    .ok_or_else(|| {
                        serde::de::Error::invalid_value(serde::de::Unexpected::Unsigned(v), &self)
                    })
            }

            fn visit_str<E>(self, value: &str) -> std::result::Result<Self::Value, E>
            where
                E: serde::de::Error,
            {
                match value {
                    "TASK_STATUS_UNSPECIFIED" => Ok(TaskStatus::Unspecified),
                    "TASK_STATUS_QUEUED" => Ok(TaskStatus::Queued),
                    "TASK_STATUS_RUNNING" => Ok(TaskStatus::Running),
                    "TASK_STATUS_SUCCEEDED" => Ok(TaskStatus::Succeeded),
                    "TASK_STATUS_FAILED" => Ok(TaskStatus::Failed),
                    "TASK_STATUS_CANCELED" => Ok(TaskStatus::Canceled),
                    _ => Err(serde::de::Error::unknown_variant(value, FIELDS)),
                }
            }
        }
        deserializer.deserialize_any(GeneratedVisitor)
    }
}
impl serde::Serialize for TaskType {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        let variant = match self {
            Self::Unspecified => "TASK_TYPE_UNSPECIFIED",
            Self::Replay => "TASK_TYPE_REPLAY",
            Self::Train => "TASK_TYPE_TRAIN",
            Self::Eval => "TASK_TYPE_EVAL",
            Self::PcapCut => "TASK_TYPE_PCAP_CUT",
        };
        serializer.serialize_str(variant)
    }
}
impl<'de> serde::Deserialize<'de> for TaskType {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "TASK_TYPE_UNSPECIFIED",
            "TASK_TYPE_REPLAY",
            "TASK_TYPE_TRAIN",
            "TASK_TYPE_EVAL",
            "TASK_TYPE_PCAP_CUT",
        ];

        struct GeneratedVisitor;

        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = TaskType;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                write!(formatter, "expected one of: {:?}", &FIELDS)
            }

            fn visit_i64<E>(self, v: i64) -> std::result::Result<Self::Value, E>
            where
                E: serde::de::Error,
            {
                i32::try_from(v)
                    .ok()
                    .and_then(|x| x.try_into().ok())
                    .ok_or_else(|| {
                        serde::de::Error::invalid_value(serde::de::Unexpected::Signed(v), &self)
                    })
            }

            fn visit_u64<E>(self, v: u64) -> std::result::Result<Self::Value, E>
            where
                E: serde::de::Error,
            {
                i32::try_from(v)
                    .ok()
                    .and_then(|x| x.try_into().ok())
                    .ok_or_else(|| {
                        serde::de::Error::invalid_value(serde::de::Unexpected::Unsigned(v), &self)
                    })
            }

            fn visit_str<E>(self, value: &str) -> std::result::Result<Self::Value, E>
            where
                E: serde::de::Error,
            {
                match value {
                    "TASK_TYPE_UNSPECIFIED" => Ok(TaskType::Unspecified),
                    "TASK_TYPE_REPLAY" => Ok(TaskType::Replay),
                    "TASK_TYPE_TRAIN" => Ok(TaskType::Train),
                    "TASK_TYPE_EVAL" => Ok(TaskType::Eval),
                    "TASK_TYPE_PCAP_CUT" => Ok(TaskType::PcapCut),
                    _ => Err(serde::de::Error::unknown_variant(value, FIELDS)),
                }
            }
        }
        deserializer.deserialize_any(GeneratedVisitor)
    }
}
impl serde::Serialize for UploadFlowsRequest {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.events.is_empty() {
            len += 1;
        }
        if !self.compression.is_empty() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.UploadFlowsRequest", len)?;
        if !self.events.is_empty() {
            struct_ser.serialize_field("events", &self.events)?;
        }
        if !self.compression.is_empty() {
            struct_ser.serialize_field("compression", &self.compression)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for UploadFlowsRequest {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "events",
            "compression",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Events,
            Compression,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "events" => Ok(GeneratedField::Events),
                            "compression" => Ok(GeneratedField::Compression),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = UploadFlowsRequest;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.UploadFlowsRequest")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<UploadFlowsRequest, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut events__ = None;
                let mut compression__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Events => {
                            if events__.is_some() {
                                return Err(serde::de::Error::duplicate_field("events"));
                            }
                            events__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Compression => {
                            if compression__.is_some() {
                                return Err(serde::de::Error::duplicate_field("compression"));
                            }
                            compression__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(UploadFlowsRequest {
                    events: events__.unwrap_or_default(),
                    compression: compression__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.UploadFlowsRequest", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for UploadFlowsResponse {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if self.accepted != 0 {
            len += 1;
        }
        if self.rejected != 0 {
            len += 1;
        }
        if !self.rejected_ids.is_empty() {
            len += 1;
        }
        if !self.message.is_empty() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.UploadFlowsResponse", len)?;
        if self.accepted != 0 {
            struct_ser.serialize_field("accepted", &self.accepted)?;
        }
        if self.rejected != 0 {
            struct_ser.serialize_field("rejected", &self.rejected)?;
        }
        if !self.rejected_ids.is_empty() {
            struct_ser.serialize_field("rejectedIds", &self.rejected_ids)?;
        }
        if !self.message.is_empty() {
            struct_ser.serialize_field("message", &self.message)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for UploadFlowsResponse {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "accepted",
            "rejected",
            "rejected_ids",
            "rejectedIds",
            "message",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Accepted,
            Rejected,
            RejectedIds,
            Message,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "accepted" => Ok(GeneratedField::Accepted),
                            "rejected" => Ok(GeneratedField::Rejected),
                            "rejectedIds" | "rejected_ids" => Ok(GeneratedField::RejectedIds),
                            "message" => Ok(GeneratedField::Message),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = UploadFlowsResponse;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.UploadFlowsResponse")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<UploadFlowsResponse, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut accepted__ = None;
                let mut rejected__ = None;
                let mut rejected_ids__ = None;
                let mut message__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Accepted => {
                            if accepted__.is_some() {
                                return Err(serde::de::Error::duplicate_field("accepted"));
                            }
                            accepted__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::Rejected => {
                            if rejected__.is_some() {
                                return Err(serde::de::Error::duplicate_field("rejected"));
                            }
                            rejected__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::RejectedIds => {
                            if rejected_ids__.is_some() {
                                return Err(serde::de::Error::duplicate_field("rejectedIds"));
                            }
                            rejected_ids__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Message => {
                            if message__.is_some() {
                                return Err(serde::de::Error::duplicate_field("message"));
                            }
                            message__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(UploadFlowsResponse {
                    accepted: accepted__.unwrap_or_default(),
                    rejected: rejected__.unwrap_or_default(),
                    rejected_ids: rejected_ids__.unwrap_or_default(),
                    message: message__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.UploadFlowsResponse", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for UploadPcapIndexRequest {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if self.index.is_some() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.UploadPcapIndexRequest", len)?;
        if let Some(v) = self.index.as_ref() {
            struct_ser.serialize_field("index", v)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for UploadPcapIndexRequest {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "index",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Index,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "index" => Ok(GeneratedField::Index),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = UploadPcapIndexRequest;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.UploadPcapIndexRequest")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<UploadPcapIndexRequest, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut index__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Index => {
                            if index__.is_some() {
                                return Err(serde::de::Error::duplicate_field("index"));
                            }
                            index__ = map_.next_value()?;
                        }
                    }
                }
                Ok(UploadPcapIndexRequest {
                    index: index__,
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.UploadPcapIndexRequest", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for UploadPcapIndexResponse {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if self.success {
            len += 1;
        }
        if !self.message.is_empty() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.UploadPcapIndexResponse", len)?;
        if self.success {
            struct_ser.serialize_field("success", &self.success)?;
        }
        if !self.message.is_empty() {
            struct_ser.serialize_field("message", &self.message)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for UploadPcapIndexResponse {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "success",
            "message",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Success,
            Message,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "success" => Ok(GeneratedField::Success),
                            "message" => Ok(GeneratedField::Message),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = UploadPcapIndexResponse;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.UploadPcapIndexResponse")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<UploadPcapIndexResponse, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut success__ = None;
                let mut message__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Success => {
                            if success__.is_some() {
                                return Err(serde::de::Error::duplicate_field("success"));
                            }
                            success__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Message => {
                            if message__.is_some() {
                                return Err(serde::de::Error::duplicate_field("message"));
                            }
                            message__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(UploadPcapIndexResponse {
                    success: success__.unwrap_or_default(),
                    message: message__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.UploadPcapIndexResponse", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for UploadSessionsRequest {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.sessions.is_empty() {
            len += 1;
        }
        if !self.compression.is_empty() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.UploadSessionsRequest", len)?;
        if !self.sessions.is_empty() {
            struct_ser.serialize_field("sessions", &self.sessions)?;
        }
        if !self.compression.is_empty() {
            struct_ser.serialize_field("compression", &self.compression)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for UploadSessionsRequest {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "sessions",
            "compression",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Sessions,
            Compression,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "sessions" => Ok(GeneratedField::Sessions),
                            "compression" => Ok(GeneratedField::Compression),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = UploadSessionsRequest;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.UploadSessionsRequest")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<UploadSessionsRequest, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut sessions__ = None;
                let mut compression__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Sessions => {
                            if sessions__.is_some() {
                                return Err(serde::de::Error::duplicate_field("sessions"));
                            }
                            sessions__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Compression => {
                            if compression__.is_some() {
                                return Err(serde::de::Error::duplicate_field("compression"));
                            }
                            compression__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(UploadSessionsRequest {
                    sessions: sessions__.unwrap_or_default(),
                    compression: compression__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.UploadSessionsRequest", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for UploadSessionsResponse {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if self.accepted != 0 {
            len += 1;
        }
        if self.rejected != 0 {
            len += 1;
        }
        if !self.rejected_ids.is_empty() {
            len += 1;
        }
        if !self.message.is_empty() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.UploadSessionsResponse", len)?;
        if self.accepted != 0 {
            struct_ser.serialize_field("accepted", &self.accepted)?;
        }
        if self.rejected != 0 {
            struct_ser.serialize_field("rejected", &self.rejected)?;
        }
        if !self.rejected_ids.is_empty() {
            struct_ser.serialize_field("rejectedIds", &self.rejected_ids)?;
        }
        if !self.message.is_empty() {
            struct_ser.serialize_field("message", &self.message)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for UploadSessionsResponse {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "accepted",
            "rejected",
            "rejected_ids",
            "rejectedIds",
            "message",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Accepted,
            Rejected,
            RejectedIds,
            Message,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "accepted" => Ok(GeneratedField::Accepted),
                            "rejected" => Ok(GeneratedField::Rejected),
                            "rejectedIds" | "rejected_ids" => Ok(GeneratedField::RejectedIds),
                            "message" => Ok(GeneratedField::Message),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = UploadSessionsResponse;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.UploadSessionsResponse")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<UploadSessionsResponse, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut accepted__ = None;
                let mut rejected__ = None;
                let mut rejected_ids__ = None;
                let mut message__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Accepted => {
                            if accepted__.is_some() {
                                return Err(serde::de::Error::duplicate_field("accepted"));
                            }
                            accepted__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::Rejected => {
                            if rejected__.is_some() {
                                return Err(serde::de::Error::duplicate_field("rejected"));
                            }
                            rejected__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::RejectedIds => {
                            if rejected_ids__.is_some() {
                                return Err(serde::de::Error::duplicate_field("rejectedIds"));
                            }
                            rejected_ids__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Message => {
                            if message__.is_some() {
                                return Err(serde::de::Error::duplicate_field("message"));
                            }
                            message__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(UploadSessionsResponse {
                    accepted: accepted__.unwrap_or_default(),
                    rejected: rejected__.unwrap_or_default(),
                    rejected_ids: rejected_ids__.unwrap_or_default(),
                    message: message__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.UploadSessionsResponse", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for UpsertAssetRequest {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if self.asset.is_some() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.UpsertAssetRequest", len)?;
        if let Some(v) = self.asset.as_ref() {
            struct_ser.serialize_field("asset", v)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for UpsertAssetRequest {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "asset",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Asset,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "asset" => Ok(GeneratedField::Asset),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = UpsertAssetRequest;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.UpsertAssetRequest")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<UpsertAssetRequest, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut asset__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Asset => {
                            if asset__.is_some() {
                                return Err(serde::de::Error::duplicate_field("asset"));
                            }
                            asset__ = map_.next_value()?;
                        }
                    }
                }
                Ok(UpsertAssetRequest {
                    asset: asset__,
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.UpsertAssetRequest", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for UpsertAssetResponse {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.asset_id.is_empty() {
            len += 1;
        }
        if self.created {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.UpsertAssetResponse", len)?;
        if !self.asset_id.is_empty() {
            struct_ser.serialize_field("assetId", &self.asset_id)?;
        }
        if self.created {
            struct_ser.serialize_field("created", &self.created)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for UpsertAssetResponse {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "asset_id",
            "assetId",
            "created",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            AssetId,
            Created,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "assetId" | "asset_id" => Ok(GeneratedField::AssetId),
                            "created" => Ok(GeneratedField::Created),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = UpsertAssetResponse;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.UpsertAssetResponse")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<UpsertAssetResponse, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut asset_id__ = None;
                let mut created__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::AssetId => {
                            if asset_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("assetId"));
                            }
                            asset_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Created => {
                            if created__.is_some() {
                                return Err(serde::de::Error::duplicate_field("created"));
                            }
                            created__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(UpsertAssetResponse {
                    asset_id: asset_id__.unwrap_or_default(),
                    created: created__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.UpsertAssetResponse", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for UserEvent {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.event_id.is_empty() {
            len += 1;
        }
        if !self.tenant_id.is_empty() {
            len += 1;
        }
        if !self.user_id.is_empty() {
            len += 1;
        }
        if !self.username.is_empty() {
            len += 1;
        }
        if !self.event_type.is_empty() {
            len += 1;
        }
        if !self.source_ip.is_empty() {
            len += 1;
        }
        if !self.user_agent.is_empty() {
            len += 1;
        }
        if !self.resource.is_empty() {
            len += 1;
        }
        if !self.action.is_empty() {
            len += 1;
        }
        if !self.result.is_empty() {
            len += 1;
        }
        if self.timestamp != 0 {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.UserEvent", len)?;
        if !self.event_id.is_empty() {
            struct_ser.serialize_field("eventId", &self.event_id)?;
        }
        if !self.tenant_id.is_empty() {
            struct_ser.serialize_field("tenantId", &self.tenant_id)?;
        }
        if !self.user_id.is_empty() {
            struct_ser.serialize_field("userId", &self.user_id)?;
        }
        if !self.username.is_empty() {
            struct_ser.serialize_field("username", &self.username)?;
        }
        if !self.event_type.is_empty() {
            struct_ser.serialize_field("eventType", &self.event_type)?;
        }
        if !self.source_ip.is_empty() {
            struct_ser.serialize_field("sourceIp", &self.source_ip)?;
        }
        if !self.user_agent.is_empty() {
            struct_ser.serialize_field("userAgent", &self.user_agent)?;
        }
        if !self.resource.is_empty() {
            struct_ser.serialize_field("resource", &self.resource)?;
        }
        if !self.action.is_empty() {
            struct_ser.serialize_field("action", &self.action)?;
        }
        if !self.result.is_empty() {
            struct_ser.serialize_field("result", &self.result)?;
        }
        if self.timestamp != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("timestamp", ToString::to_string(&self.timestamp).as_str())?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for UserEvent {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "event_id",
            "eventId",
            "tenant_id",
            "tenantId",
            "user_id",
            "userId",
            "username",
            "event_type",
            "eventType",
            "source_ip",
            "sourceIp",
            "user_agent",
            "userAgent",
            "resource",
            "action",
            "result",
            "timestamp",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            EventId,
            TenantId,
            UserId,
            Username,
            EventType,
            SourceIp,
            UserAgent,
            Resource,
            Action,
            Result,
            Timestamp,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "eventId" | "event_id" => Ok(GeneratedField::EventId),
                            "tenantId" | "tenant_id" => Ok(GeneratedField::TenantId),
                            "userId" | "user_id" => Ok(GeneratedField::UserId),
                            "username" => Ok(GeneratedField::Username),
                            "eventType" | "event_type" => Ok(GeneratedField::EventType),
                            "sourceIp" | "source_ip" => Ok(GeneratedField::SourceIp),
                            "userAgent" | "user_agent" => Ok(GeneratedField::UserAgent),
                            "resource" => Ok(GeneratedField::Resource),
                            "action" => Ok(GeneratedField::Action),
                            "result" => Ok(GeneratedField::Result),
                            "timestamp" => Ok(GeneratedField::Timestamp),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = UserEvent;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.UserEvent")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<UserEvent, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut event_id__ = None;
                let mut tenant_id__ = None;
                let mut user_id__ = None;
                let mut username__ = None;
                let mut event_type__ = None;
                let mut source_ip__ = None;
                let mut user_agent__ = None;
                let mut resource__ = None;
                let mut action__ = None;
                let mut result__ = None;
                let mut timestamp__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::EventId => {
                            if event_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("eventId"));
                            }
                            event_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::TenantId => {
                            if tenant_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("tenantId"));
                            }
                            tenant_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::UserId => {
                            if user_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("userId"));
                            }
                            user_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Username => {
                            if username__.is_some() {
                                return Err(serde::de::Error::duplicate_field("username"));
                            }
                            username__ = Some(map_.next_value()?);
                        }
                        GeneratedField::EventType => {
                            if event_type__.is_some() {
                                return Err(serde::de::Error::duplicate_field("eventType"));
                            }
                            event_type__ = Some(map_.next_value()?);
                        }
                        GeneratedField::SourceIp => {
                            if source_ip__.is_some() {
                                return Err(serde::de::Error::duplicate_field("sourceIp"));
                            }
                            source_ip__ = Some(map_.next_value()?);
                        }
                        GeneratedField::UserAgent => {
                            if user_agent__.is_some() {
                                return Err(serde::de::Error::duplicate_field("userAgent"));
                            }
                            user_agent__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Resource => {
                            if resource__.is_some() {
                                return Err(serde::de::Error::duplicate_field("resource"));
                            }
                            resource__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Action => {
                            if action__.is_some() {
                                return Err(serde::de::Error::duplicate_field("action"));
                            }
                            action__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Result => {
                            if result__.is_some() {
                                return Err(serde::de::Error::duplicate_field("result"));
                            }
                            result__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Timestamp => {
                            if timestamp__.is_some() {
                                return Err(serde::de::Error::duplicate_field("timestamp"));
                            }
                            timestamp__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                    }
                }
                Ok(UserEvent {
                    event_id: event_id__.unwrap_or_default(),
                    tenant_id: tenant_id__.unwrap_or_default(),
                    user_id: user_id__.unwrap_or_default(),
                    username: username__.unwrap_or_default(),
                    event_type: event_type__.unwrap_or_default(),
                    source_ip: source_ip__.unwrap_or_default(),
                    user_agent: user_agent__.unwrap_or_default(),
                    resource: resource__.unwrap_or_default(),
                    action: action__.unwrap_or_default(),
                    result: result__.unwrap_or_default(),
                    timestamp: timestamp__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.UserEvent", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for UserEventBatch {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.events.is_empty() {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.UserEventBatch", len)?;
        if !self.events.is_empty() {
            struct_ser.serialize_field("events", &self.events)?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for UserEventBatch {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "events",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            Events,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "events" => Ok(GeneratedField::Events),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = UserEventBatch;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.UserEventBatch")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<UserEventBatch, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut events__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::Events => {
                            if events__.is_some() {
                                return Err(serde::de::Error::duplicate_field("events"));
                            }
                            events__ = Some(map_.next_value()?);
                        }
                    }
                }
                Ok(UserEventBatch {
                    events: events__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.UserEventBatch", FIELDS, GeneratedVisitor)
    }
}
impl serde::Serialize for WhitelistRule {
    #[allow(deprecated)]
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut len = 0;
        if !self.tenant_id.is_empty() {
            len += 1;
        }
        if !self.rule_id.is_empty() {
            len += 1;
        }
        if !self.rule_type.is_empty() {
            len += 1;
        }
        if !self.src_ip.is_empty() {
            len += 1;
        }
        if !self.dst_ip.is_empty() {
            len += 1;
        }
        if self.src_port != 0 {
            len += 1;
        }
        if self.dst_port != 0 {
            len += 1;
        }
        if self.protocol != 0 {
            len += 1;
        }
        if !self.alert_type.is_empty() {
            len += 1;
        }
        if !self.reason_code.is_empty() {
            len += 1;
        }
        if !self.comment.is_empty() {
            len += 1;
        }
        if !self.status.is_empty() {
            len += 1;
        }
        if !self.created_by.is_empty() {
            len += 1;
        }
        if self.created_ts != 0 {
            len += 1;
        }
        if self.updated_ts != 0 {
            len += 1;
        }
        if self.expires_at != 0 {
            len += 1;
        }
        if self.ingest_ts != 0 {
            len += 1;
        }
        let mut struct_ser = serializer.serialize_struct("traffic.v1.WhitelistRule", len)?;
        if !self.tenant_id.is_empty() {
            struct_ser.serialize_field("tenantId", &self.tenant_id)?;
        }
        if !self.rule_id.is_empty() {
            struct_ser.serialize_field("ruleId", &self.rule_id)?;
        }
        if !self.rule_type.is_empty() {
            struct_ser.serialize_field("ruleType", &self.rule_type)?;
        }
        if !self.src_ip.is_empty() {
            struct_ser.serialize_field("srcIp", &self.src_ip)?;
        }
        if !self.dst_ip.is_empty() {
            struct_ser.serialize_field("dstIp", &self.dst_ip)?;
        }
        if self.src_port != 0 {
            struct_ser.serialize_field("srcPort", &self.src_port)?;
        }
        if self.dst_port != 0 {
            struct_ser.serialize_field("dstPort", &self.dst_port)?;
        }
        if self.protocol != 0 {
            struct_ser.serialize_field("protocol", &self.protocol)?;
        }
        if !self.alert_type.is_empty() {
            struct_ser.serialize_field("alertType", &self.alert_type)?;
        }
        if !self.reason_code.is_empty() {
            struct_ser.serialize_field("reasonCode", &self.reason_code)?;
        }
        if !self.comment.is_empty() {
            struct_ser.serialize_field("comment", &self.comment)?;
        }
        if !self.status.is_empty() {
            struct_ser.serialize_field("status", &self.status)?;
        }
        if !self.created_by.is_empty() {
            struct_ser.serialize_field("createdBy", &self.created_by)?;
        }
        if self.created_ts != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("createdTs", ToString::to_string(&self.created_ts).as_str())?;
        }
        if self.updated_ts != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("updatedTs", ToString::to_string(&self.updated_ts).as_str())?;
        }
        if self.expires_at != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("expiresAt", ToString::to_string(&self.expires_at).as_str())?;
        }
        if self.ingest_ts != 0 {
            #[allow(clippy::needless_borrow)]
            #[allow(clippy::needless_borrows_for_generic_args)]
            struct_ser.serialize_field("ingestTs", ToString::to_string(&self.ingest_ts).as_str())?;
        }
        struct_ser.end()
    }
}
impl<'de> serde::Deserialize<'de> for WhitelistRule {
    #[allow(deprecated)]
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        const FIELDS: &[&str] = &[
            "tenant_id",
            "tenantId",
            "rule_id",
            "ruleId",
            "rule_type",
            "ruleType",
            "src_ip",
            "srcIp",
            "dst_ip",
            "dstIp",
            "src_port",
            "srcPort",
            "dst_port",
            "dstPort",
            "protocol",
            "alert_type",
            "alertType",
            "reason_code",
            "reasonCode",
            "comment",
            "status",
            "created_by",
            "createdBy",
            "created_ts",
            "createdTs",
            "updated_ts",
            "updatedTs",
            "expires_at",
            "expiresAt",
            "ingest_ts",
            "ingestTs",
        ];

        #[allow(clippy::enum_variant_names)]
        enum GeneratedField {
            TenantId,
            RuleId,
            RuleType,
            SrcIp,
            DstIp,
            SrcPort,
            DstPort,
            Protocol,
            AlertType,
            ReasonCode,
            Comment,
            Status,
            CreatedBy,
            CreatedTs,
            UpdatedTs,
            ExpiresAt,
            IngestTs,
        }
        impl<'de> serde::Deserialize<'de> for GeneratedField {
            fn deserialize<D>(deserializer: D) -> std::result::Result<GeneratedField, D::Error>
            where
                D: serde::Deserializer<'de>,
            {
                struct GeneratedVisitor;

                impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
                    type Value = GeneratedField;

                    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(formatter, "expected one of: {:?}", &FIELDS)
                    }

                    #[allow(unused_variables)]
                    fn visit_str<E>(self, value: &str) -> std::result::Result<GeneratedField, E>
                    where
                        E: serde::de::Error,
                    {
                        match value {
                            "tenantId" | "tenant_id" => Ok(GeneratedField::TenantId),
                            "ruleId" | "rule_id" => Ok(GeneratedField::RuleId),
                            "ruleType" | "rule_type" => Ok(GeneratedField::RuleType),
                            "srcIp" | "src_ip" => Ok(GeneratedField::SrcIp),
                            "dstIp" | "dst_ip" => Ok(GeneratedField::DstIp),
                            "srcPort" | "src_port" => Ok(GeneratedField::SrcPort),
                            "dstPort" | "dst_port" => Ok(GeneratedField::DstPort),
                            "protocol" => Ok(GeneratedField::Protocol),
                            "alertType" | "alert_type" => Ok(GeneratedField::AlertType),
                            "reasonCode" | "reason_code" => Ok(GeneratedField::ReasonCode),
                            "comment" => Ok(GeneratedField::Comment),
                            "status" => Ok(GeneratedField::Status),
                            "createdBy" | "created_by" => Ok(GeneratedField::CreatedBy),
                            "createdTs" | "created_ts" => Ok(GeneratedField::CreatedTs),
                            "updatedTs" | "updated_ts" => Ok(GeneratedField::UpdatedTs),
                            "expiresAt" | "expires_at" => Ok(GeneratedField::ExpiresAt),
                            "ingestTs" | "ingest_ts" => Ok(GeneratedField::IngestTs),
                            _ => Err(serde::de::Error::unknown_field(value, FIELDS)),
                        }
                    }
                }
                deserializer.deserialize_identifier(GeneratedVisitor)
            }
        }
        struct GeneratedVisitor;
        impl<'de> serde::de::Visitor<'de> for GeneratedVisitor {
            type Value = WhitelistRule;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("struct traffic.v1.WhitelistRule")
            }

            fn visit_map<V>(self, mut map_: V) -> std::result::Result<WhitelistRule, V::Error>
                where
                    V: serde::de::MapAccess<'de>,
            {
                let mut tenant_id__ = None;
                let mut rule_id__ = None;
                let mut rule_type__ = None;
                let mut src_ip__ = None;
                let mut dst_ip__ = None;
                let mut src_port__ = None;
                let mut dst_port__ = None;
                let mut protocol__ = None;
                let mut alert_type__ = None;
                let mut reason_code__ = None;
                let mut comment__ = None;
                let mut status__ = None;
                let mut created_by__ = None;
                let mut created_ts__ = None;
                let mut updated_ts__ = None;
                let mut expires_at__ = None;
                let mut ingest_ts__ = None;
                while let Some(k) = map_.next_key()? {
                    match k {
                        GeneratedField::TenantId => {
                            if tenant_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("tenantId"));
                            }
                            tenant_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::RuleId => {
                            if rule_id__.is_some() {
                                return Err(serde::de::Error::duplicate_field("ruleId"));
                            }
                            rule_id__ = Some(map_.next_value()?);
                        }
                        GeneratedField::RuleType => {
                            if rule_type__.is_some() {
                                return Err(serde::de::Error::duplicate_field("ruleType"));
                            }
                            rule_type__ = Some(map_.next_value()?);
                        }
                        GeneratedField::SrcIp => {
                            if src_ip__.is_some() {
                                return Err(serde::de::Error::duplicate_field("srcIp"));
                            }
                            src_ip__ = Some(map_.next_value()?);
                        }
                        GeneratedField::DstIp => {
                            if dst_ip__.is_some() {
                                return Err(serde::de::Error::duplicate_field("dstIp"));
                            }
                            dst_ip__ = Some(map_.next_value()?);
                        }
                        GeneratedField::SrcPort => {
                            if src_port__.is_some() {
                                return Err(serde::de::Error::duplicate_field("srcPort"));
                            }
                            src_port__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::DstPort => {
                            if dst_port__.is_some() {
                                return Err(serde::de::Error::duplicate_field("dstPort"));
                            }
                            dst_port__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::Protocol => {
                            if protocol__.is_some() {
                                return Err(serde::de::Error::duplicate_field("protocol"));
                            }
                            protocol__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::AlertType => {
                            if alert_type__.is_some() {
                                return Err(serde::de::Error::duplicate_field("alertType"));
                            }
                            alert_type__ = Some(map_.next_value()?);
                        }
                        GeneratedField::ReasonCode => {
                            if reason_code__.is_some() {
                                return Err(serde::de::Error::duplicate_field("reasonCode"));
                            }
                            reason_code__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Comment => {
                            if comment__.is_some() {
                                return Err(serde::de::Error::duplicate_field("comment"));
                            }
                            comment__ = Some(map_.next_value()?);
                        }
                        GeneratedField::Status => {
                            if status__.is_some() {
                                return Err(serde::de::Error::duplicate_field("status"));
                            }
                            status__ = Some(map_.next_value()?);
                        }
                        GeneratedField::CreatedBy => {
                            if created_by__.is_some() {
                                return Err(serde::de::Error::duplicate_field("createdBy"));
                            }
                            created_by__ = Some(map_.next_value()?);
                        }
                        GeneratedField::CreatedTs => {
                            if created_ts__.is_some() {
                                return Err(serde::de::Error::duplicate_field("createdTs"));
                            }
                            created_ts__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::UpdatedTs => {
                            if updated_ts__.is_some() {
                                return Err(serde::de::Error::duplicate_field("updatedTs"));
                            }
                            updated_ts__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::ExpiresAt => {
                            if expires_at__.is_some() {
                                return Err(serde::de::Error::duplicate_field("expiresAt"));
                            }
                            expires_at__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                        GeneratedField::IngestTs => {
                            if ingest_ts__.is_some() {
                                return Err(serde::de::Error::duplicate_field("ingestTs"));
                            }
                            ingest_ts__ = 
                                Some(map_.next_value::<::pbjson::private::NumberDeserialize<_>>()?.0)
                            ;
                        }
                    }
                }
                Ok(WhitelistRule {
                    tenant_id: tenant_id__.unwrap_or_default(),
                    rule_id: rule_id__.unwrap_or_default(),
                    rule_type: rule_type__.unwrap_or_default(),
                    src_ip: src_ip__.unwrap_or_default(),
                    dst_ip: dst_ip__.unwrap_or_default(),
                    src_port: src_port__.unwrap_or_default(),
                    dst_port: dst_port__.unwrap_or_default(),
                    protocol: protocol__.unwrap_or_default(),
                    alert_type: alert_type__.unwrap_or_default(),
                    reason_code: reason_code__.unwrap_or_default(),
                    comment: comment__.unwrap_or_default(),
                    status: status__.unwrap_or_default(),
                    created_by: created_by__.unwrap_or_default(),
                    created_ts: created_ts__.unwrap_or_default(),
                    updated_ts: updated_ts__.unwrap_or_default(),
                    expires_at: expires_at__.unwrap_or_default(),
                    ingest_ts: ingest_ts__.unwrap_or_default(),
                })
            }
        }
        deserializer.deserialize_struct("traffic.v1.WhitelistRule", FIELDS, GeneratedVisitor)
    }
}
