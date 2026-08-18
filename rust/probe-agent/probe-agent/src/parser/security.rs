use md5;
use sha2::{Digest, Sha256};

const TLS_HANDSHAKE: u8 = 22;
const CLIENT_HELLO: u8 = 1;
const CERTIFICATE: u8 = 11;

#[derive(Clone, Copy, Debug, PartialEq, Eq, PartialOrd, Ord)]
pub enum ObservedSecurityProtocol {
    Tls,
    Quic,
}

#[derive(Clone, Debug, Default, PartialEq, Eq)]
pub struct SecurityObservation {
    pub protocol: Option<ObservedSecurityProtocol>,
    pub tls_version: Option<String>,
    pub ja3: Option<String>,
    pub ja4: Option<String>,
    pub sni: Option<String>,
    pub cert_sha256: Option<String>,
    pub cert_is_self_signed: Option<bool>,
    pub pubkey_len: Option<u32>,
    pub quic_version: Option<String>,
    pub truncated: bool,
}

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct PacketFeatureObservation {
    pub signed_packet_length: i32,
    pub event_time_us: i64,
    pub payload_nibble_counts: [u64; 16],
    pub payload_observed_bytes: u64,
    pub security: SecurityObservation,
}

impl PacketFeatureObservation {
    pub fn from_decoded_frame(
        fields: &super::FlowFields,
        direction: crate::aggregator::PacketDirection,
        event_time_us: u64,
        frame: &[u8],
    ) -> Self {
        let payload = frame
            .get(fields.application_payload_offset..fields.application_payload_end)
            .unwrap_or_default();
        let mut payload_nibble_counts = [0u64; 16];
        for byte in payload {
            payload_nibble_counts[(byte >> 4) as usize] += 1;
            payload_nibble_counts[(byte & 0x0f) as usize] += 1;
        }
        let packet_length = i32::from(fields.total_len);
        Self {
            signed_packet_length: if direction.is_forward() {
                packet_length
            } else {
                -packet_length
            },
            event_time_us: event_time_us.min(i64::MAX as u64) as i64,
            payload_nibble_counts,
            payload_observed_bytes: payload.len() as u64,
            security: observe_transport_security(
                fields.protocol,
                fields.src_port,
                fields.dst_port,
                payload,
            ),
        }
    }
}

#[derive(Debug)]
struct ClientHello {
    legacy_version: u16,
    ciphers: Vec<u16>,
    extensions: Vec<u16>,
    supported_groups: Vec<u16>,
    point_formats: Vec<u8>,
    supported_versions: Vec<u16>,
    signature_algorithms: Vec<u16>,
    sni: Option<String>,
    alpn: Option<String>,
}

pub fn observe_transport_security(
    protocol: u8,
    src_port: u16,
    dst_port: u16,
    payload: &[u8],
) -> SecurityObservation {
    if protocol == 17 && looks_like_quic_long_header(payload) {
        return observe_quic(payload);
    }
    if protocol != 6 || payload.is_empty() {
        return SecurityObservation::default();
    }

    // A port is only a hint used after a TLS record signature is present.
    // Non-standard TLS ports remain supported and port 443 alone is not TLS.
    if !looks_like_tls_record(payload) {
        let _ = (src_port, dst_port);
        return SecurityObservation::default();
    }

    observe_tls(payload)
}

fn looks_like_tls_record(payload: &[u8]) -> bool {
    payload.len() >= 5 && matches!(payload[0], 20..=23) && payload[1] == 3 && payload[2] <= 4
}

fn looks_like_quic_long_header(payload: &[u8]) -> bool {
    payload.len() >= 5 && payload[0] & 0xc0 == 0xc0
}

fn observe_quic(payload: &[u8]) -> SecurityObservation {
    let version = u32::from_be_bytes([payload[1], payload[2], payload[3], payload[4]]);
    SecurityObservation {
        protocol: Some(ObservedSecurityProtocol::Quic),
        quic_version: Some(format!("0x{version:08x}")),
        ..SecurityObservation::default()
    }
}

fn observe_tls(payload: &[u8]) -> SecurityObservation {
    let mut observation = SecurityObservation {
        protocol: Some(ObservedSecurityProtocol::Tls),
        ..SecurityObservation::default()
    };
    let mut cursor = 0usize;
    while cursor + 5 <= payload.len() {
        let content_type = payload[cursor];
        let record_version = u16::from_be_bytes([payload[cursor + 1], payload[cursor + 2]]);
        let record_len = u16::from_be_bytes([payload[cursor + 3], payload[cursor + 4]]) as usize;
        let record_start = cursor + 5;
        let Some(record_end) = record_start.checked_add(record_len) else {
            observation.truncated = true;
            break;
        };
        if record_end > payload.len() {
            observation.truncated = true;
            break;
        }
        if observation.tls_version.is_none() {
            observation.tls_version = Some(tls_version_name(record_version));
        }
        if content_type == TLS_HANDSHAKE {
            parse_handshakes(&payload[record_start..record_end], &mut observation);
        }
        cursor = record_end;
    }
    observation
}

fn parse_handshakes(mut bytes: &[u8], observation: &mut SecurityObservation) {
    while !bytes.is_empty() {
        if bytes.len() < 4 {
            observation.truncated = true;
            return;
        }
        let handshake_type = bytes[0];
        let len = read_u24(&bytes[1..4]);
        if bytes.len() < 4 + len {
            observation.truncated = true;
            return;
        }
        let body = &bytes[4..4 + len];
        match handshake_type {
            CLIENT_HELLO => match parse_client_hello(body) {
                Some(hello) => {
                    observation.tls_version = Some(tls_version_name(
                        hello
                            .supported_versions
                            .iter()
                            .copied()
                            .filter(|version| !is_grease(*version))
                            .max()
                            .unwrap_or(hello.legacy_version),
                    ));
                    observation.ja3 = Some(ja3(&hello));
                    observation.ja4 = Some(ja4(&hello));
                    observation.sni = hello.sni;
                }
                None => observation.truncated = true,
            },
            CERTIFICATE => {
                if let Some(cert) = first_tls12_certificate(body) {
                    observation.cert_sha256 = Some(sha256_hex(cert));
                    let (self_signed, pubkey_len) = inspect_certificate_der(cert);
                    observation.cert_is_self_signed = self_signed;
                    observation.pubkey_len = pubkey_len;
                } else {
                    observation.truncated = true;
                }
            }
            _ => {}
        }
        bytes = &bytes[4 + len..];
    }
}

fn parse_client_hello(body: &[u8]) -> Option<ClientHello> {
    if body.len() < 35 {
        return None;
    }
    let legacy_version = u16::from_be_bytes([body[0], body[1]]);
    let mut cursor = 34usize;
    let session_len = *body.get(cursor)? as usize;
    cursor = cursor.checked_add(1 + session_len)?;
    let cipher_len = read_u16_at(body, cursor)? as usize;
    cursor += 2;
    if cipher_len % 2 != 0 || cursor + cipher_len > body.len() {
        return None;
    }
    let ciphers = read_u16_list(&body[cursor..cursor + cipher_len]);
    cursor += cipher_len;
    let compression_len = *body.get(cursor)? as usize;
    cursor = cursor.checked_add(1 + compression_len)?;

    let mut hello = ClientHello {
        legacy_version,
        ciphers,
        extensions: Vec::new(),
        supported_groups: Vec::new(),
        point_formats: Vec::new(),
        supported_versions: Vec::new(),
        signature_algorithms: Vec::new(),
        sni: None,
        alpn: None,
    };
    if cursor == body.len() {
        return Some(hello);
    }
    let extensions_len = read_u16_at(body, cursor)? as usize;
    cursor += 2;
    if cursor + extensions_len != body.len() {
        return None;
    }
    let end = cursor + extensions_len;
    while cursor < end {
        let extension_type = read_u16_at(body, cursor)?;
        let extension_len = read_u16_at(body, cursor + 2)? as usize;
        cursor += 4;
        if cursor + extension_len > end {
            return None;
        }
        let extension = &body[cursor..cursor + extension_len];
        hello.extensions.push(extension_type);
        match extension_type {
            0 => hello.sni = parse_sni(extension),
            10 => hello.supported_groups = parse_prefixed_u16_list(extension).unwrap_or_default(),
            11 => hello.point_formats = parse_prefixed_u8_list(extension).unwrap_or_default(),
            13 => {
                hello.signature_algorithms = parse_prefixed_u16_list(extension).unwrap_or_default()
            }
            16 => hello.alpn = parse_alpn(extension),
            43 => {
                hello.supported_versions = parse_prefixed_u8_u16_list(extension).unwrap_or_default()
            }
            _ => {}
        }
        cursor += extension_len;
    }
    Some(hello)
}

fn ja3(hello: &ClientHello) -> String {
    let canonical = format!(
        "{},{},{},{},{}",
        hello.legacy_version,
        join_u16(
            hello
                .ciphers
                .iter()
                .copied()
                .filter(|value| !is_grease(*value))
        ),
        join_u16(
            hello
                .extensions
                .iter()
                .copied()
                .filter(|value| !is_grease(*value))
        ),
        join_u16(
            hello
                .supported_groups
                .iter()
                .copied()
                .filter(|value| !is_grease(*value)),
        ),
        hello
            .point_formats
            .iter()
            .map(u8::to_string)
            .collect::<Vec<_>>()
            .join("-")
    );
    format!("{:x}", md5::compute(canonical.as_bytes()))
}

fn ja4(hello: &ClientHello) -> String {
    let version = hello
        .supported_versions
        .iter()
        .copied()
        .filter(|value| !is_grease(*value))
        .max()
        .unwrap_or(hello.legacy_version);
    let version_code = match version {
        0x0304 => "13",
        0x0303 => "12",
        0x0302 => "11",
        0x0301 => "10",
        _ => "00",
    };
    let sni_code = if hello.sni.is_some() { 'd' } else { 'i' };
    let cipher_values: Vec<u16> = hello
        .ciphers
        .iter()
        .copied()
        .filter(|value| !is_grease(*value))
        .collect();
    let extension_values: Vec<u16> = hello
        .extensions
        .iter()
        .copied()
        .filter(|value| !is_grease(*value))
        .collect();
    let alpn_code = hello
        .alpn
        .as_deref()
        .filter(|value| !value.is_empty())
        .map(|value| {
            let mut chars = value.chars();
            let first = chars.next().unwrap_or('0');
            let last = value.chars().last().unwrap_or('0');
            format!("{first}{last}")
        })
        .unwrap_or_else(|| "00".to_string());
    let prefix = format!(
        "t{version_code}{sni_code}{:02}{:02}{alpn_code}",
        cipher_values.len().min(99),
        extension_values.len().min(99)
    );
    let cipher_hash = truncated_sha256(&sorted_hex_u16(&cipher_values));
    let mut fingerprint_extensions: Vec<u16> = extension_values
        .into_iter()
        .filter(|value| *value != 0 && *value != 16)
        .collect();
    fingerprint_extensions.sort_unstable();
    let extension_part = format!(
        "{}_{}",
        sorted_hex_u16(&fingerprint_extensions),
        hello
            .signature_algorithms
            .iter()
            .map(|value| format!("{value:04x}"))
            .collect::<Vec<_>>()
            .join(",")
    );
    format!(
        "{prefix}_{cipher_hash}_{}",
        truncated_sha256(&extension_part)
    )
}

fn parse_sni(extension: &[u8]) -> Option<String> {
    let list_len = read_u16_at(extension, 0)? as usize;
    if list_len + 2 != extension.len() {
        return None;
    }
    let mut cursor = 2usize;
    while cursor + 3 <= extension.len() {
        let name_type = extension[cursor];
        let name_len = read_u16_at(extension, cursor + 1)? as usize;
        cursor += 3;
        let name = extension.get(cursor..cursor + name_len)?;
        if name_type == 0 {
            let value = std::str::from_utf8(name)
                .ok()?
                .trim_end_matches('.')
                .to_ascii_lowercase();
            if is_valid_dns_name(&value) {
                return Some(value);
            }
            return None;
        }
        cursor += name_len;
    }
    None
}

fn is_valid_dns_name(value: &str) -> bool {
    !value.is_empty()
        && value.len() <= 253
        && value.split('.').all(|label| {
            !label.is_empty()
                && label.len() <= 63
                && !label.starts_with('-')
                && !label.ends_with('-')
                && label
                    .bytes()
                    .all(|byte| byte.is_ascii_alphanumeric() || byte == b'-')
        })
}

fn parse_alpn(extension: &[u8]) -> Option<String> {
    let list_len = read_u16_at(extension, 0)? as usize;
    if list_len + 2 != extension.len() || list_len == 0 {
        return None;
    }
    let value_len = *extension.get(2)? as usize;
    let value = extension.get(3..3 + value_len)?;
    std::str::from_utf8(value).ok().map(ToOwned::to_owned)
}

fn first_tls12_certificate(body: &[u8]) -> Option<&[u8]> {
    let list_len = read_u24(body.get(0..3)?);
    if list_len + 3 > body.len() || list_len < 3 {
        return None;
    }
    let cert_len = read_u24(body.get(3..6)?);
    body.get(6..6 + cert_len)
}

fn inspect_certificate_der(cert: &[u8]) -> (Option<bool>, Option<u32>) {
    let (_, cert_body) = der_tlv(cert, 0, 0x30).unwrap_or((cert.len(), &[]));
    let (_, tbs) = der_tlv(cert_body, 0, 0x30).unwrap_or((cert_body.len(), &[]));
    if tbs.is_empty() {
        return (None, None);
    }
    let mut cursor = 0usize;
    if tbs.get(cursor) == Some(&0xa0) {
        cursor = der_any(tbs, cursor).map(|item| item.0).unwrap_or(tbs.len());
    }
    // serialNumber and signature
    cursor = der_any(tbs, cursor).map(|item| item.0).unwrap_or(tbs.len());
    cursor = der_any(tbs, cursor).map(|item| item.0).unwrap_or(tbs.len());
    let issuer = der_any(tbs, cursor);
    let Some((next, issuer_raw)) = issuer else {
        return (None, None);
    };
    cursor = next;
    cursor = der_any(tbs, cursor).map(|item| item.0).unwrap_or(tbs.len()); // validity
    let Some((next, subject_raw)) = der_any(tbs, cursor) else {
        return (None, None);
    };
    cursor = next;
    let Some((_, spki)) = der_tlv(tbs, cursor, 0x30) else {
        return (Some(issuer_raw == subject_raw), None);
    };
    let Some((after_algorithm, _)) = der_any(spki, 0) else {
        return (Some(issuer_raw == subject_raw), None);
    };
    let pubkey_len = der_tlv(spki, after_algorithm, 0x03).and_then(|(_, bit_string)| {
        let unused = *bit_string.first()? as u32;
        Some(((bit_string.len().saturating_sub(1) as u32) * 8).saturating_sub(unused))
    });
    (Some(issuer_raw == subject_raw), pubkey_len)
}

fn der_any(bytes: &[u8], offset: usize) -> Option<(usize, &[u8])> {
    let tag = *bytes.get(offset)?;
    der_tlv(bytes, offset, tag)
}

fn der_tlv(bytes: &[u8], offset: usize, expected_tag: u8) -> Option<(usize, &[u8])> {
    if *bytes.get(offset)? != expected_tag {
        return None;
    }
    let first_len = *bytes.get(offset + 1)?;
    let (header_len, value_len) = if first_len & 0x80 == 0 {
        (2usize, first_len as usize)
    } else {
        let count = (first_len & 0x7f) as usize;
        if count == 0 || count > 4 {
            return None;
        }
        let mut len = 0usize;
        for byte in bytes.get(offset + 2..offset + 2 + count)? {
            len = len.checked_mul(256)?.checked_add(*byte as usize)?;
        }
        (2 + count, len)
    };
    let start = offset.checked_add(header_len)?;
    let end = start.checked_add(value_len)?;
    Some((end, bytes.get(start..end)?))
}

fn parse_prefixed_u16_list(bytes: &[u8]) -> Option<Vec<u16>> {
    let len = read_u16_at(bytes, 0)? as usize;
    let values = bytes.get(2..2 + len)?;
    if len + 2 != bytes.len() || len % 2 != 0 {
        return None;
    }
    Some(read_u16_list(values))
}

fn parse_prefixed_u8_u16_list(bytes: &[u8]) -> Option<Vec<u16>> {
    let len = *bytes.first()? as usize;
    let values = bytes.get(1..1 + len)?;
    if len + 1 != bytes.len() || len % 2 != 0 {
        return None;
    }
    Some(read_u16_list(values))
}

fn parse_prefixed_u8_list(bytes: &[u8]) -> Option<Vec<u8>> {
    let len = *bytes.first()? as usize;
    if len + 1 != bytes.len() {
        return None;
    }
    Some(bytes[1..].to_vec())
}

fn read_u16_list(bytes: &[u8]) -> Vec<u16> {
    bytes
        .chunks_exact(2)
        .map(|item| u16::from_be_bytes([item[0], item[1]]))
        .collect()
}

fn read_u16_at(bytes: &[u8], offset: usize) -> Option<u16> {
    Some(u16::from_be_bytes([
        *bytes.get(offset)?,
        *bytes.get(offset + 1)?,
    ]))
}

fn read_u24(bytes: &[u8]) -> usize {
    (bytes[0] as usize) << 16 | (bytes[1] as usize) << 8 | bytes[2] as usize
}

fn join_u16(values: impl Iterator<Item = u16>) -> String {
    values
        .map(|value| value.to_string())
        .collect::<Vec<_>>()
        .join("-")
}

fn is_grease(value: u16) -> bool {
    value & 0x0f0f == 0x0a0a && value >> 8 == value & 0xff
}

fn sorted_hex_u16(values: &[u16]) -> String {
    let mut values = values.to_vec();
    values.sort_unstable();
    values
        .iter()
        .map(|value| format!("{value:04x}"))
        .collect::<Vec<_>>()
        .join(",")
}

fn truncated_sha256(value: &str) -> String {
    sha256_hex(value.as_bytes())[..12].to_string()
}

fn sha256_hex(bytes: &[u8]) -> String {
    Sha256::digest(bytes)
        .iter()
        .map(|byte| format!("{byte:02x}"))
        .collect()
}

fn tls_version_name(version: u16) -> String {
    match version {
        0x0304 => "TLS1.3".to_string(),
        0x0303 => "TLS1.2".to_string(),
        0x0302 => "TLS1.1".to_string(),
        0x0301 => "TLS1.0".to_string(),
        _ => format!("0x{version:04x}"),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn client_hello() -> Vec<u8> {
        let sni = b"example.com";
        let mut extensions = Vec::new();
        let mut sni_body = vec![0, (sni.len() + 3) as u8, 0, 0, sni.len() as u8];
        sni_body.extend_from_slice(sni);
        extensions.extend_from_slice(&[0, 0, 0, sni_body.len() as u8]);
        extensions.extend_from_slice(&sni_body);
        extensions.extend_from_slice(&[0, 10, 0, 6, 0, 4, 0, 29, 0, 23]);
        extensions.extend_from_slice(&[0, 11, 0, 2, 1, 0]);
        extensions.extend_from_slice(&[0, 13, 0, 6, 0, 4, 4, 3, 8, 4]);
        extensions.extend_from_slice(&[0, 16, 0, 5, 0, 3, 2, b'h', b'2']);
        extensions.extend_from_slice(&[0, 43, 0, 5, 4, 3, 4, 3, 3]);

        let mut hello = vec![3, 3];
        hello.extend_from_slice(&[0; 32]);
        hello.push(0);
        hello.extend_from_slice(&[0, 6, 0x13, 1, 0x13, 2, 0x0a, 0x0a]);
        hello.extend_from_slice(&[1, 0]);
        hello.extend_from_slice(&(extensions.len() as u16).to_be_bytes());
        hello.extend_from_slice(&extensions);

        let mut handshake = vec![CLIENT_HELLO];
        let len = hello.len();
        handshake.extend_from_slice(&[
            ((len >> 16) & 0xff) as u8,
            ((len >> 8) & 0xff) as u8,
            len as u8,
        ]);
        handshake.extend_from_slice(&hello);
        let mut record = vec![TLS_HANDSHAKE, 3, 1];
        record.extend_from_slice(&(handshake.len() as u16).to_be_bytes());
        record.extend_from_slice(&handshake);
        record
    }

    #[test]
    fn client_hello_has_stable_real_ja3_ja4_and_sni() {
        let first = observe_transport_security(6, 50_000, 443, &client_hello());
        let second = observe_transport_security(6, 50_000, 443, &client_hello());
        assert_eq!(first, second);
        assert_eq!(first.protocol, Some(ObservedSecurityProtocol::Tls));
        assert_eq!(first.tls_version.as_deref(), Some("TLS1.3"));
        assert_eq!(first.sni.as_deref(), Some("example.com"));
        assert_eq!(first.ja3.as_deref().map(str::len), Some(32));
        assert_eq!(first.ja4.as_deref().map(str::len), Some(36));
        assert!(!first.truncated);
    }

    #[test]
    fn tls_port_without_record_is_not_called_encrypted() {
        assert_eq!(
            observe_transport_security(6, 50_000, 443, b"GET / HTTP/1.1\r\n\r\n"),
            SecurityObservation::default()
        );
    }

    #[test]
    fn quic_long_header_records_only_observable_version() {
        let observed = observe_transport_security(17, 50_000, 443, &[0xc3, 0, 0, 0, 1, 8, 0]);
        assert_eq!(observed.protocol, Some(ObservedSecurityProtocol::Quic));
        assert_eq!(observed.quic_version.as_deref(), Some("0x00000001"));
        assert!(observed.ja3.is_none());
    }

    #[test]
    fn truncated_tls_is_explicit() {
        let observed = observe_transport_security(6, 1, 2, &[22, 3, 3, 0, 8, 1]);
        assert_eq!(observed.protocol, Some(ObservedSecurityProtocol::Tls));
        assert!(observed.truncated);
    }
}
