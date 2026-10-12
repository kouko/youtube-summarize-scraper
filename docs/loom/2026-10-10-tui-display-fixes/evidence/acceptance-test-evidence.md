# acceptance-test-evidence.md

## TestModelCToggleRawYAML

Command:
```
go test -race ./tui -run TestModelCToggleRawYAML -count=1
```

Result: PASS

Test steps verified:
1. Create temporary config file with YAML content `llm:\n  provider: claude-api\n`
2. Start TUI model, select the config file
3. Verify initial mode is `ConfigViewStructured`
4. Press 'c' key, verify mode changes to `ConfigViewRaw`
5. Verify rendered view contains raw YAML content ("llm:" and "provider: claude-api")
6. Press 'c' again, verify mode changes back to `ConfigViewStructured`
7. Verify rendered view no longer contains raw YAML verbatim

## TestModelConfigTableCJKWidth

Command:
```
go test -race ./tui -run TestModelConfigTableCJKWidth -count=1
```

Result: PASS

Test steps verified:
1. Create ConfigView with YAML containing CJK characters: `playlists:\n  - name: "稍後觀看"\n    count: 10\n`
2. Render table with width 60
3. Verify all rows have display width equal to border row width (60)
4. Row 6 (the CJK row) previously had width 68 (misaligned), now has width 60 (aligned)

## Full package suite

Command:
```
go test ./... -count=1 -timeout 120s
```

Result: All packages PASS