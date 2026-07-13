# UI Desktop Capture Receiver Self-test

- Result: `pass`
- Generated: `2026-07-03T09:59:11.321963+08:00`
- Expected viewport: `1920x1080`
- Checks passed: `13/13`

This self-test uses a temporary evidence directory. It does not replace Desktop Chrome screenshots, `capture-meta.json`, visual diff metrics, or route `interaction.json` evidence.

## Checks

- `pass` health endpoint responds: status=200
- `pass` viewport probe page is served: status=200 contains_report_endpoint=True
- `pass` token endpoint rejects unauthenticated reads: status=403
- `pass` token endpoint accepts capture key: status=200
- `pass` viewport report pass is stored: status=201 result=pass
- `pass` viewport report blocked is stored: status=201 result=blocked viewport={'width': 2560, 'height': 1271}
- `pass` viewport report rejects sensitive material: status=400 preserved_result=blocked
- `pass` screenshot upload writes passing capture metadata: status=201 meta_status=pass
- `pass` interaction screenshot upload writes passing metadata: status=201 exists=True meta_status=pass
- `pass` interaction upload writes JSON: status=201 exists=True
- `pass` interaction upload rejects sensitive material: status=400
- `pass` bridge result upload writes run summary: status=201 result=pass
- `pass` bridge result upload rejects sensitive material: status=400
