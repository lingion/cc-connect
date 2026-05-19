# Feishu Fixtures

Run `docs/FIXTURE-COLLECTION.md` to collect fixtures from the live service.

## Naming convention

```
YYYYMMDD_HHMMSS_NNN_<type>_<msg_id_tail>.json
```

- `type` = text | image | file | audio | mixed | unknown
- `NNN`  = sequence number within the collection session

## Required minimum set (for CI)

| File pattern       | Description                        |
|--------------------|------------------------------------|
| `*_text_*.json`    | Private chat – plain text          |
| `*_text_group*.json` | Group chat – @mention + text     |
| `*_image_*.json`   | Private chat – image attachment    |
| `*_file_*.json`    | Private chat – file attachment     |

Place at least one file per type here before enabling platform_sim tests.
