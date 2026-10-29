
## 操作步骤（固定流程）

```bash
SL=./shellink   # 需在本目录（shellink-cli/）下执行

# 1. [Claude] 用已保存的 profile 每次新建会话（不复用旧会话）
"$SL" session create --profile 'ccec0d97-2317-4c30-b0b9-a7527df5a281' --json

# 2. [Claude] 切 MANUAL，然后等待用户
"$SL" session mode '<session-id>' --mode MANUAL --json

# 3. [用户] 另开终端运行 "$SL" cli，打开该会话，
#    在 GateShell 菜单中登录目标服务器，完成后告知 Claude

# 4. [Claude] 收到确认后切回 AUTO
"$SL" session mode '<session-id>' --mode AUTO --json

# 5. [Claude] 继续执行命令
"$SL" session exec '<session-id>' --command 'uname -a' --json
```
