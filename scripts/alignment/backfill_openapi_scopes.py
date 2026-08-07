#!/usr/bin/env python3
"""Add or verify the T-GW-001 required-scope extensions without reformatting OpenAPI."""

from __future__ import annotations

import argparse
import json
import re
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
OPENAPI = ROOT / "contracts/openapi/alignment-v1.openapi.json"
SCOPES = {
    "cancelAlertResponseAction": "playbook:execute",
    "createAlertReport": "alert:export",
    "createAlertResponseAction": "alert:write",
    "downloadAlertReport": "alert:export",
    "getAlertReportJob": "alert:export",
    "getAlertEvidence": "alert:read",
    "getAlertResponseAction": "playbook:read",
    "listAlertViews": "alert:read",
    "linkAlertToCampaign": "campaign:write",
    "listAlertCampaignLinks": "campaign:read",
    "unlinkAlertFromCampaign": "campaign:write",
    "changeAuthenticatedUserPasswordAtomic": "user:write",
    "updateAuthenticatedUserProfileAtomic": "user:write",
    "exploreBoundedGraph": "graph:read",
    "cancelAssetDiscoveryJob": "asset:discover",
    "createAssetDiscoveryJob": "asset:discover",
    "getAssetDiscoveryJob": "asset:read",
    "listAssetDiscoveryCandidates": "asset:read",
    "listAssetDiscoveryJobHistory": "asset:read",
    "listAssetDiscoveryJobs": "asset:read",
    "listAssetsV2": "asset:read",
    "upsertAssetV2": "asset:govern",
    "cancelCampaignSOARJob": "playbook:execute",
    "compensateCampaignSOARJob": "playbook:approve",
    "decideCampaignSOARJob": "playbook:approve",
    "downloadCampaignReport": "campaign:report",
    "getCampaign": "campaign:read",
    "getCampaignCommandJob": "campaign:read",
    "getCampaignReport": "campaign:report",
    "getCampaignSOARJob": "playbook:read",
    "listCampaignMembers": "campaign:read",
    "listCampaigns": "campaign:read",
    "submitCampaignCommand": "campaign:write",
    "acknowledgeProbeOperation": "probe:ingest",
    "getProbeOperation": "probe:read",
    "pushProbeConfig": "probe:write",
    "saveAlertView": "alert:write",
    "createTopicActionJob": "topic:write",
    "getTopicActionJob": "topic:read",
    "getTopicSnapshot": "topic:read",
}
OPERATION = re.compile(r'^(?P<indent>\s*)"operationId": "(?P<operation>[^"]+)",$')


def rendered() -> tuple[str, list[str]]:
    lines = OPENAPI.read_text(encoding="utf-8").splitlines()
    found: set[str] = set()
    output: list[str] = []
    for index, line in enumerate(lines):
        output.append(line)
        match = OPERATION.match(line)
        if not match or match.group("operation") not in SCOPES:
            continue
        operation = match.group("operation")
        found.add(operation)
        scope_line = f'{match.group("indent")}"x-required-scope": "{SCOPES[operation]}",'
        if index + 1 >= len(lines) or lines[index + 1] != scope_line:
            output.append(scope_line)
    missing = sorted(set(SCOPES) - found)
    return "\n".join(output) + "\n", missing


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--check", action="store_true")
    args = parser.parse_args()
    value, missing = rendered()
    current = OPENAPI.read_text(encoding="utf-8")
    if missing:
        print(json.dumps({"status": "FAIL", "missing_operation_ids": missing}, indent=2))
        return 1
    if args.check:
        status = "PASS" if current == value else "FAIL"
        print(json.dumps({"status": status, "governed_operations": len(SCOPES)}, indent=2))
        return 0 if status == "PASS" else 1
    OPENAPI.write_text(value, encoding="utf-8")
    print(json.dumps({"status": "UPDATED", "governed_operations": len(SCOPES)}, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
