use anyhow::{bail, Context, Result};
use serde::{Deserialize, Serialize};
use sha2::{Digest, Sha256};
use std::ffi::CString;
use std::io::ErrorKind;
use std::os::unix::ffi::OsStrExt;
use std::path::{Path, PathBuf};
use std::sync::Arc;
use tokio::io::AsyncWriteExt;

use super::buffer::UploadData;
use super::pcap::PcapGlobalHeader;
use super::upload_journal::UploadJournal;

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct JournaledUploadRef {
    pub task_id: String,
    pub tenant_id: String,
    pub probe_id: String,
    pub capture_uuid: String,
    pub manifest_hash: String,
    pub object_key: String,
    pub spool_root: PathBuf,
    pub local_path: PathBuf,
    pub original_size: u64,
    pub stored_size: u64,
    pub sha256: String,
    pub ts_start: u64,
    pub ts_end: u64,
    pub packet_count: u64,
}

impl JournaledUploadRef {
    pub fn validate_identity(&self) -> Result<()> {
        validate_identity_component("tenant_id", &self.tenant_id)?;
        validate_identity_component("probe_id", &self.probe_id)?;
        if uuid::Uuid::parse_str(&self.capture_uuid).is_err()
            || uuid::Uuid::parse_str(&self.task_id).is_err()
        {
            bail!("spool capture/task identity is not a UUID");
        }
        validate_sha256("manifest_hash", &self.manifest_hash)?;
        validate_sha256("sha256", &self.sha256)?;
        if self.object_key.trim().is_empty()
            || !self
                .object_key
                .starts_with(&format!("{}/{}/", self.tenant_id, self.probe_id))
        {
            bail!("spool object key conflicts with tenant/probe identity");
        }
        if self.original_size == 0
            || self.stored_size == 0
            || self.packet_count == 0
            || self.ts_start == 0
            || self.ts_end < self.ts_start
        {
            bail!("spool manifest contains invalid size, packet count, or time range");
        }
        Ok(())
    }
}

#[derive(Clone)]
pub struct DurablePcapSpool {
    root: PathBuf,
    journal: Arc<UploadJournal>,
    zstd_level: i32,
}

#[derive(Debug, Clone, Default, PartialEq, Eq)]
pub struct SpoolReconcileReport {
    pub final_files_inspected: usize,
    pub pending_records_rebuilt: usize,
    pub owned_temp_files_removed: usize,
}

impl DurablePcapSpool {
    pub fn new(root: PathBuf, journal: Arc<UploadJournal>, zstd_level: i32) -> Result<Self> {
        if !(-7..=22).contains(&zstd_level) {
            bail!("zstd level {zstd_level} is outside the supported range -7..=22");
        }
        std::fs::create_dir_all(&root)
            .with_context(|| format!("failed to create spool root {}", root.display()))?;
        let root = std::fs::canonicalize(&root)
            .with_context(|| format!("failed to canonicalize spool root {}", root.display()))?;
        Ok(Self {
            root,
            journal,
            zstd_level,
        })
    }

    pub async fn persist_rotated(
        &self,
        upload: UploadData,
        tenant_id: &str,
        probe_id: &str,
    ) -> Result<JournaledUploadRef> {
        validate_identity_component("tenant_id", tenant_id)?;
        validate_identity_component("probe_id", probe_id)?;
        validate_packet_records(&upload)?;

        let mut pcap = Vec::with_capacity(PcapGlobalHeader::size() + upload.data.len());
        PcapGlobalHeader::default().write_to(&mut pcap)?;
        pcap.extend_from_slice(&upload.data);
        let pcap_len = pcap.len();
        let zstd_level = self.zstd_level;
        // zstd compression and SHA-256 are CPU-bound: run them on a blocking
        // thread so the async rotator/uploader tasks are never stalled.
        let (original_sha256, compressed, sha256) =
            tokio::task::spawn_blocking(move || {
                let original_sha256 = sha256_hex(&pcap);
                let compressed = zstd::encode_all(&pcap[..], zstd_level)
                    .context("failed to compress immutable PCAP spool")?;
                let sha256 = sha256_hex(&compressed);
                Ok::<_, anyhow::Error>((original_sha256, compressed, sha256))
            })
            .await
            .context("PCAP spool compression task panicked")??;

        let capture_identity = format!(
            "{tenant_id}\0{probe_id}\0{}\0{}\0{}\0{original_sha256}",
            upload.ts_start, upload.ts_end, upload.packet_count
        );
        let capture_uuid =
            uuid::Uuid::new_v5(&uuid::Uuid::NAMESPACE_OID, capture_identity.as_bytes()).to_string();
        let object_key = format!("{tenant_id}/{probe_id}/{capture_uuid}.pcap.zst");
        let manifest_hash = sha256_hex(
            format!(
                "v1\0{tenant_id}\0{probe_id}\0{capture_uuid}\0{object_key}\0{}\0{}\0{}\0{}\0{}\0{sha256}",
                pcap_len,
                compressed.len(),
                upload.ts_start,
                upload.ts_end,
                upload.packet_count
            )
            .as_bytes(),
        );
        let task_id = uuid::Uuid::new_v5(
            &uuid::Uuid::NAMESPACE_OID,
            format!("pcap-spool-journal\0{capture_uuid}\0{manifest_hash}").as_bytes(),
        )
        .to_string();
        let final_directory = self.root.join(tenant_id).join(probe_id);
        tokio::fs::create_dir_all(&final_directory).await?;
        let final_directory = tokio::fs::canonicalize(&final_directory).await?;
        if !final_directory.starts_with(&self.root) {
            bail!("spool identity directory escapes the canonical root");
        }
        sync_directory_chain(&self.root, &final_directory).await?;
        let final_path = final_directory.join(format!("{capture_uuid}-{manifest_hash}.pcap.zst"));

        if tokio::fs::try_exists(&final_path).await? {
            verify_existing_spool(&final_path, compressed.len(), &sha256).await?;
        } else {
            self.publish_durable_file(&final_directory, &final_path, &compressed)
                .await?;
        }

        let upload_ref = JournaledUploadRef {
            task_id,
            tenant_id: tenant_id.to_string(),
            probe_id: probe_id.to_string(),
            capture_uuid,
            manifest_hash,
            object_key,
            spool_root: self.root.clone(),
            local_path: final_path,
            original_size: pcap_len as u64,
            stored_size: compressed.len() as u64,
            sha256,
            ts_start: upload.ts_start,
            ts_end: upload.ts_end,
            packet_count: upload.packet_count,
        };
        self.journal.record_spooled_pending(&upload_ref)?;
        Ok(upload_ref)
    }

    pub fn root(&self) -> &Path {
        &self.root
    }

    pub async fn reconcile_orphans(&self) -> Result<SpoolReconcileReport> {
        // The whole pass is synchronous disk I/O plus zstd decode; run it on a
        // blocking thread so startup recovery never stalls the async runtime.
        let root = self.root.clone();
        let journal = self.journal.clone();
        tokio::task::spawn_blocking(move || {
            let mut report = SpoolReconcileReport::default();
            for path in collect_spool_files(&root)? {
                let name = path
                    .file_name()
                    .context("spool entry has no file name")?
                    .to_string_lossy();
                if name.starts_with('.') && name.ends_with(".tmp") {
                    let metadata = std::fs::symlink_metadata(&path)?;
                    if metadata.file_type().is_symlink() || !metadata.is_file() {
                        bail!("owned spool temp is not a regular file: {}", path.display());
                    }
                    std::fs::remove_file(&path)?;
                    if let Some(parent) = path.parent() {
                        std::fs::File::open(parent)?.sync_all()?;
                    }
                    report.owned_temp_files_removed += 1;
                    continue;
                }
                if name.starts_with(".cleanup-") {
                    // The cleanup reconciler owns rename-before-unlink artifacts
                    // and validates them against the durable cleanup claim.
                    continue;
                }
                if !name.ends_with(".pcap.zst") {
                    bail!("unexpected durable spool entry: {}", path.display());
                }
                report.final_files_inspected += 1;
                let upload = reconstruct_upload_ref(&root, &path)?;
                let existed = journal.get_entry(&upload.task_id)?.is_some();
                journal.record_spooled_pending(&upload)?;
                if !existed {
                    report.pending_records_rebuilt += 1;
                }
            }
            Ok::<_, anyhow::Error>(report)
        })
        .await
        .context("spool orphan reconciliation task panicked")?
    }

    async fn publish_durable_file(
        &self,
        final_directory: &Path,
        final_path: &Path,
        bytes: &[u8],
    ) -> Result<()> {
        let temp_path = final_directory.join(format!(
            ".{}.{}.tmp",
            final_path
                .file_name()
                .context("spool final path has no file name")?
                .to_string_lossy(),
            uuid::Uuid::new_v4()
        ));
        let mut temp_guard = OwnedTempFile::new(temp_path.clone());
        let mut file = tokio::fs::OpenOptions::new()
            .create_new(true)
            .write(true)
            .open(&temp_path)
            .await
            .with_context(|| {
                format!("failed to create unique spool temp {}", temp_path.display())
            })?;
        file.write_all(bytes).await?;
        file.sync_data().await?;
        drop(file);

        match rename_noreplace(&temp_path, final_path) {
            Ok(()) => temp_guard.disarm(),
            Err(error) if error.kind() == ErrorKind::AlreadyExists => {
                verify_existing_spool(final_path, bytes.len(), &sha256_hex(bytes)).await?;
            }
            Err(error) => return Err(error).context("failed to atomically publish PCAP spool"),
        }
        sync_parent_directory(final_directory).await?;
        Ok(())
    }
}

struct OwnedTempFile {
    path: PathBuf,
    armed: bool,
}

impl OwnedTempFile {
    fn new(path: PathBuf) -> Self {
        Self { path, armed: true }
    }

    fn disarm(&mut self) {
        self.armed = false;
    }
}

impl Drop for OwnedTempFile {
    fn drop(&mut self) {
        if self.armed {
            let _ = std::fs::remove_file(&self.path);
        }
    }
}

fn rename_noreplace(source: &Path, destination: &Path) -> std::io::Result<()> {
    let source = CString::new(source.as_os_str().as_bytes())?;
    let destination = CString::new(destination.as_os_str().as_bytes())?;
    let result = unsafe {
        libc::syscall(
            libc::SYS_renameat2,
            libc::AT_FDCWD,
            source.as_ptr(),
            libc::AT_FDCWD,
            destination.as_ptr(),
            libc::RENAME_NOREPLACE,
        )
    };
    if result == 0 {
        Ok(())
    } else {
        Err(std::io::Error::last_os_error())
    }
}

async fn sync_parent_directory(root: &Path) -> Result<()> {
    let root = root.to_path_buf();
    let sync_root = root.clone();
    tokio::task::spawn_blocking(move || {
        let directory = std::fs::File::open(&sync_root)?;
        directory.sync_all()
    })
    .await
    .context("parent directory sync task failed")?
    .with_context(|| format!("failed to fsync spool parent {}", root.display()))
}

async fn sync_directory_chain(root: &Path, leaf: &Path) -> Result<()> {
    sync_parent_directory(root).await?;
    if let Some(parent) = leaf.parent() {
        if parent != root {
            sync_parent_directory(parent).await?;
        }
    }
    if leaf != root {
        sync_parent_directory(leaf).await?;
    }
    Ok(())
}

fn collect_spool_files(root: &Path) -> Result<Vec<PathBuf>> {
    fn visit(root: &Path, directory: &Path, files: &mut Vec<PathBuf>) -> Result<()> {
        for entry in std::fs::read_dir(directory)? {
            let entry = entry?;
            let path = entry.path();
            let metadata = std::fs::symlink_metadata(&path)?;
            if metadata.file_type().is_symlink() {
                bail!(
                    "symlink found beneath durable spool root: {}",
                    path.display()
                );
            }
            if metadata.is_dir() {
                let canonical = std::fs::canonicalize(&path)?;
                if !canonical.starts_with(root) {
                    bail!("spool directory escapes canonical root: {}", path.display());
                }
                visit(root, &canonical, files)?;
            } else if metadata.is_file() {
                files.push(path);
            } else {
                bail!(
                    "non-regular entry found beneath durable spool root: {}",
                    path.display()
                );
            }
        }
        Ok(())
    }

    let mut files = Vec::new();
    visit(root, root, &mut files)?;
    files.sort();
    Ok(files)
}

fn reconstruct_upload_ref(root: &Path, path: &Path) -> Result<JournaledUploadRef> {
    let canonical_path = std::fs::canonicalize(path)?;
    if !canonical_path.starts_with(root) {
        bail!("orphan spool path escapes canonical root");
    }
    let relative = canonical_path.strip_prefix(root)?;
    let components: Vec<_> = relative.components().collect();
    if components.len() != 3 {
        bail!("orphan spool path does not encode tenant/probe identity");
    }
    let tenant_id = components[0].as_os_str().to_string_lossy().into_owned();
    let probe_id = components[1].as_os_str().to_string_lossy().into_owned();
    validate_identity_component("tenant_id", &tenant_id)?;
    validate_identity_component("probe_id", &probe_id)?;
    let file_name = components[2].as_os_str().to_string_lossy();
    let stem = file_name
        .strip_suffix(".pcap.zst")
        .context("orphan spool file does not have the canonical suffix")?;
    if stem.len() != 36 + 1 + 64 || &stem[36..37] != "-" {
        bail!("orphan spool file does not encode capture and manifest identity");
    }
    let encoded_capture_uuid = &stem[..36];
    let encoded_manifest_hash = &stem[37..];
    uuid::Uuid::parse_str(encoded_capture_uuid).context("invalid orphan capture UUID")?;
    validate_sha256("manifest_hash", encoded_manifest_hash)?;

    let compressed = std::fs::read(&canonical_path)?;
    let pcap = zstd::decode_all(&compressed[..]).context("orphan spool zstd decode failed")?;
    let mut expected_header = Vec::new();
    PcapGlobalHeader::default().write_to(&mut expected_header)?;
    if pcap.len() <= expected_header.len() || !pcap.starts_with(&expected_header) {
        bail!("orphan spool has no canonical PCAP global header");
    }
    let records = &pcap[expected_header.len()..];
    let (packet_count, ts_start, ts_end) = inspect_packet_records(records)?;
    let original_sha256 = sha256_hex(&pcap);
    let capture_identity =
        format!("{tenant_id}\0{probe_id}\0{ts_start}\0{ts_end}\0{packet_count}\0{original_sha256}");
    let capture_uuid =
        uuid::Uuid::new_v5(&uuid::Uuid::NAMESPACE_OID, capture_identity.as_bytes()).to_string();
    if capture_uuid != encoded_capture_uuid {
        bail!("orphan spool capture UUID conflicts with its PCAP contents");
    }
    let object_key = format!("{tenant_id}/{probe_id}/{capture_uuid}.pcap.zst");
    let sha256 = sha256_hex(&compressed);
    let manifest_hash = sha256_hex(
        format!(
            "v1\0{tenant_id}\0{probe_id}\0{capture_uuid}\0{object_key}\0{}\0{}\0{ts_start}\0{ts_end}\0{packet_count}\0{sha256}",
            pcap.len(),
            compressed.len()
        )
        .as_bytes(),
    );
    if manifest_hash != encoded_manifest_hash {
        bail!("orphan spool manifest hash conflicts with its durable contents");
    }
    let task_id = uuid::Uuid::new_v5(
        &uuid::Uuid::NAMESPACE_OID,
        format!("pcap-spool-journal\0{capture_uuid}\0{manifest_hash}").as_bytes(),
    )
    .to_string();
    let upload = JournaledUploadRef {
        task_id,
        tenant_id,
        probe_id,
        capture_uuid,
        manifest_hash,
        object_key,
        spool_root: root.to_path_buf(),
        local_path: canonical_path,
        original_size: pcap.len() as u64,
        stored_size: compressed.len() as u64,
        sha256,
        ts_start,
        ts_end,
        packet_count,
    };
    upload.validate_identity()?;
    Ok(upload)
}

async fn verify_existing_spool(
    path: &Path,
    expected_size: usize,
    expected_sha256: &str,
) -> Result<()> {
    let metadata = tokio::fs::symlink_metadata(path).await?;
    if metadata.file_type().is_symlink() || !metadata.is_file() {
        bail!("existing spool is not a regular file");
    }
    if metadata.len() != expected_size as u64 {
        bail!("stable spool identity conflicts with an existing file size");
    }
    let bytes = tokio::fs::read(path).await?;
    if sha256_hex(&bytes) != expected_sha256 {
        bail!("stable spool identity conflicts with an existing file hash");
    }
    Ok(())
}

fn validate_packet_records(upload: &UploadData) -> Result<()> {
    if upload.data.is_empty()
        || upload.packet_count == 0
        || upload.ts_start == 0
        || upload.ts_end < upload.ts_start
    {
        bail!("rotated upload has invalid bytes, count, or time range");
    }
    let (count, ts_start, ts_end) = inspect_packet_records(&upload.data)?;
    if count != upload.packet_count || ts_start != upload.ts_start || ts_end != upload.ts_end {
        bail!("rotated PCAP records differ from the count/time manifest");
    }
    Ok(())
}

pub fn inspect_packet_records(data: &[u8]) -> Result<(u64, u64, u64)> {
    let mut cursor = 0usize;
    let mut count = 0u64;
    let mut first = None;
    let mut last = None;
    while cursor < data.len() {
        if data.len() - cursor < 16 {
            bail!("rotated upload ends with a partial PCAP packet header");
        }
        let ts_sec = u32::from_le_bytes(data[cursor..cursor + 4].try_into()?);
        let ts_usec = u32::from_le_bytes(data[cursor + 4..cursor + 8].try_into()?);
        let incl_len = u32::from_le_bytes(data[cursor + 8..cursor + 12].try_into()?) as usize;
        let orig_len = u32::from_le_bytes(data[cursor + 12..cursor + 16].try_into()?) as usize;
        if incl_len == 0 || incl_len > orig_len || ts_usec >= 1_000_000 {
            bail!("rotated upload contains an invalid PCAP packet header");
        }
        cursor = cursor
            .checked_add(16 + incl_len)
            .context("PCAP record length overflow")?;
        if cursor > data.len() {
            bail!("rotated upload ends with a partial PCAP packet payload");
        }
        let timestamp = ts_sec as u64 * 1_000_000 + ts_usec as u64;
        first.get_or_insert(timestamp);
        last = Some(timestamp);
        count += 1;
    }
    Ok((
        count,
        first.context("PCAP has no packet records")?,
        last.context("PCAP has no packet records")?,
    ))
}

fn validate_identity_component(name: &str, value: &str) -> Result<()> {
    if value.is_empty()
        || !value.chars().all(|character| {
            character.is_ascii_alphanumeric() || character == '-' || character == '_'
        })
    {
        bail!("{name} contains invalid characters");
    }
    Ok(())
}

fn validate_sha256(name: &str, value: &str) -> Result<()> {
    if value.len() != 64 || !value.bytes().all(|byte| byte.is_ascii_hexdigit()) {
        bail!("{name} is not a lowercase SHA-256 hex digest");
    }
    Ok(())
}

fn sha256_hex(bytes: &[u8]) -> String {
    let mut hasher = Sha256::new();
    hasher.update(bytes);
    format!("{:x}", hasher.finalize())
}

#[cfg(test)]
mod tests {
    use super::{DurablePcapSpool, UploadData};
    use crate::archiver::UploadJournal;
    use std::sync::Arc;

    fn packet(timestamp: u64, payload: &[u8]) -> Vec<u8> {
        let mut record = Vec::new();
        record.extend_from_slice(&((timestamp / 1_000_000) as u32).to_le_bytes());
        record.extend_from_slice(&((timestamp % 1_000_000) as u32).to_le_bytes());
        record.extend_from_slice(&(payload.len() as u32).to_le_bytes());
        record.extend_from_slice(&(payload.len() as u32).to_le_bytes());
        record.extend_from_slice(payload);
        record
    }

    #[tokio::test]
    async fn durable_spool_is_valid_pcap_and_idempotent() {
        let directory = tempfile::tempdir().expect("directory");
        let journal = Arc::new(UploadJournal::new(directory.path()).expect("journal"));
        let spool =
            DurablePcapSpool::new(directory.path().join("spool"), journal, 3).expect("spool");
        let mut records = packet(1_000_001, b"one");
        records.extend(packet(2_000_002, b"two"));
        let upload = UploadData {
            data: records,
            ts_start: 1_000_001,
            ts_end: 2_000_002,
            packet_count: 2,
        };

        let first = spool
            .persist_rotated(upload.clone(), "tenant-a", "probe-a")
            .await
            .expect("first persist");
        let replay = spool
            .persist_rotated(upload, "tenant-a", "probe-a")
            .await
            .expect("idempotent persist");
        assert_eq!(first, replay);
        let compressed = tokio::fs::read(&first.local_path).await.expect("read");
        let pcap = zstd::decode_all(&compressed[..]).expect("decode");
        assert_eq!(&pcap[..4], &0xa1b2c3d4u32.to_le_bytes());
        assert_eq!(pcap.len() as u64, first.original_size);
        assert_eq!(compressed.len() as u64, first.stored_size);
        assert_eq!(
            std::fs::read_dir(first.local_path.parent().expect("parent"))
                .expect("directory")
                .filter_map(Result::ok)
                .filter(|entry| entry.file_name().to_string_lossy().ends_with(".tmp"))
                .count(),
            0
        );
    }

    #[tokio::test]
    async fn partial_packet_never_publishes_or_journals() {
        let directory = tempfile::tempdir().expect("directory");
        let journal = Arc::new(UploadJournal::new(directory.path()).expect("journal"));
        let spool = DurablePcapSpool::new(directory.path().join("spool"), journal.clone(), 3)
            .expect("spool");
        let error = spool
            .persist_rotated(
                UploadData {
                    data: vec![0; 15],
                    ts_start: 1,
                    ts_end: 1,
                    packet_count: 1,
                },
                "tenant-a",
                "probe-a",
            )
            .await
            .expect_err("partial record must fail");
        assert!(error.to_string().contains("partial PCAP packet header"));
        assert_eq!(journal.size().expect("journal size"), 0);
        assert_eq!(
            std::fs::read_dir(directory.path().join("spool"))
                .expect("spool dir")
                .count(),
            0
        );
    }

    #[tokio::test]
    async fn startup_rebuilds_final_file_without_journal_and_removes_owned_temp() {
        let directory = tempfile::tempdir().expect("directory");
        let first_journal = Arc::new(
            UploadJournal::new(directory.path().join("first-journal")).expect("first journal"),
        );
        let spool_root = directory.path().join("spool");
        let first_spool =
            DurablePcapSpool::new(spool_root.clone(), first_journal, 3).expect("first spool");
        let upload = UploadData {
            data: packet(1_000_001, b"orphan"),
            ts_start: 1_000_001,
            ts_end: 1_000_001,
            packet_count: 1,
        };
        let durable = first_spool
            .persist_rotated(upload, "tenant-a", "probe-a")
            .await
            .expect("durable file");

        let recovered_journal = Arc::new(
            UploadJournal::new(directory.path().join("recovered-journal"))
                .expect("recovered journal"),
        );
        let recovered_spool = DurablePcapSpool::new(spool_root, recovered_journal.clone(), 3)
            .expect("recovered spool");
        let owned_temp = durable
            .local_path
            .parent()
            .expect("parent")
            .join(".owned-crash-window.tmp");
        std::fs::write(&owned_temp, b"partial").expect("temp crash fixture");

        let report = recovered_spool
            .reconcile_orphans()
            .await
            .expect("orphan reconciliation");
        assert_eq!(report.final_files_inspected, 1);
        assert_eq!(report.pending_records_rebuilt, 1);
        assert_eq!(report.owned_temp_files_removed, 1);
        assert!(!owned_temp.exists());
        let rebuilt = recovered_journal
            .get_entry(&durable.task_id)
            .expect("lookup")
            .expect("rebuilt PENDING entry");
        assert_eq!(rebuilt.manifest_hash, durable.manifest_hash);
        assert_eq!(rebuilt.local_path.as_deref(), durable.local_path.to_str());
    }
}
