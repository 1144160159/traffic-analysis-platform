# Windows 聊天生图收件箱同步说明

更新时间：2026-06-20

## 1. 目录关系

GPT 聊天窗口生成图片后，下载到 Windows 本机目录：

```text
C:\Users\11441\Downloads\traffic-ui-imagegen-inbox
```

在 10.0.5.8 的项目目录中，统一拉取到：

```text
/home/wangwt/phase_2/code/traffic-analysis-platform/doc/04_assets/ui_suite_gpt_v1/inbox
```

已创建远端归档目录：

```text
C:\Users\11441\Downloads\traffic-ui-imagegen-processed
```

## 2. SSH 状态

已配置 10.0.5.8 到 10.3.6.6 的专用同步 key：

```text
/root/.ssh/id_ed25519_traffic_ui_sync
```

Windows 账号 `11441` 属于 Administrators，OpenSSH 实际读取的是：

```text
C:\ProgramData\ssh\administrators_authorized_keys
```

不是用户目录下的：

```text
C:\Users\11441\.ssh\authorized_keys
```

## 3. 使用方式

在 Windows GPT 聊天窗口下载图片时，将图片放到：

```text
C:\Users\11441\Downloads\traffic-ui-imagegen-inbox
```

然后在 10.0.5.8 项目根目录执行：

```bash
bash doc/04_assets/ui_suite_gpt_v1/pull_chat_images_from_windows.sh
```

如果希望拉取后把 Windows 收件箱里的图片移动到 processed 目录，执行：

```bash
MOVE_REMOTE=1 bash doc/04_assets/ui_suite_gpt_v1/pull_chat_images_from_windows.sh
```

## 4. 支持文件类型

当前脚本拉取以下类型：

```text
png, jpg, jpeg, webp, avif
```

## 5. 后续归档流程

拉取后，图片会先进入：

```text
doc/04_assets/ui_suite_gpt_v1/inbox
```

后续再根据 `CHAT_IMAGEGEN_INVENTORY.md` 的图片 ID 重命名并移动到：

```text
screens/foundations
screens/pages
screens/overlays
screens/components
screens/states
screens/responsive
```
