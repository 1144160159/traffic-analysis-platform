# UI 图拆解记录门禁状态

本文件只判断“逐图拆解记录”是否完整，不判断前端实现截图和 pixel diff。像素级复刻仍以 `PIXEL_PERFECT_PIPELINE_STATUS.md` 为准。

## 汇总

- 总图数：241
- 拆解通过：241
- 拆解未通过：0
- breakdown-accepted：241

## 深拆最低门槛

- 主拆解记录行数：>= 220
- review 行数：>= 35
- regions/texts/components/icons/tokens/interactions：>= 12/30/6/5/10/5
- 基准：`foundation-color-status`。本门禁不代表像素 diff 通过。

## 未通过队列

| 分类 | 图片 ID | 缺失项 |
|---|---|---|
