# 可复现随机输入与等价类策略

项目不再依赖 4 个硬编码输入值。Go Explorer 和 Java Runner 现在都从 `seed + 字段稳定身份 + 等价类名称` 生成确定性随机内容：不同 seed 扩展覆盖，同一 seed 能完整复现失败。

## 默认语料

每个安全编辑框默认生成 18 个用例：

| 类别 | 目的 |
| --- | --- |
| `empty`、`whitespace` | 空值、纯空格等价类 |
| `ascii_min`、`ascii_short` | 最小非空值、普通短文本 |
| `ascii_boundary_minus_1`、`ascii_boundary`、`ascii_boundary_plus_1` | 31/32/33 常见长度边界 |
| `ascii_max` | 配置允许的最大生成长度，默认 64 |
| `cjk`、`symbols`、`emoji`、`mixed` | 中文、特殊符号、补充平面字符、多字符集组合 |
| `numeric_zero`、`numeric_negative`、`numeric_decimal` | 数值等价类 |
| `email_valid`、`email_invalid` | 合法/非法结构化文本 |
| `leading_trailing_space` | 首尾空格与 trim 行为 |

随机字符本身不会写死；类别、边界和安全上限是稳定的测试模型。输入日志记录类别、Unicode 字符长度和 seed，以便定位与复现。

## 配置

Go Explorer：

```json
{
  "seed": 2026091003,
  "inputStrategy": {
    "casesPerField": 18,
    "maxLength": 64
  }
}
```

Java Runner：

```json
{
  "seed": 2026091003,
  "inputCasesPerField": 18,
  "inputMaxLength": 64
}
```

`casesPerField` 范围为 1～18，`maxLength` 范围为 1～256。默认分别为 18 和 64。较长内容会按 Unicode code point 截断，不会切断 Emoji 的代理对。

## 安全约束

- password、PIN、OTP、验证码、银行卡、支付等敏感字段继续被排除。
- 每字段语料量和最大长度都有硬上限，不能演变为无限随机输入。
- 输入动作仍属于状态图和全局步骤预算，会被去重和停止机制约束。
- 编辑框当前值不进入页面身份，随机内容不会制造无限“新场景”。
- 空值用例只执行清空，不调用文本注入接口。

## 真机验证

覆盖率优先模式默认每个输入框只执行 1 个正常短文本，避免单个搜索框消耗整段探索预算。`inputCasesPerField` 仍可显式设置为 1–18；设置为 18 时进入深度输入覆盖，保留空值、最小值、31/32/33/64 长度边界等完整语料。历史 18 类真机验证记录仍保存在 `tests/reports/foloy-generated-input-validation-final.json`，后续回归目标应用统一使用 Gallery 或异常 Fixture。

## 扩展原则

后续若增加手机号、URL、日期或业务专用格式，应新增命名等价类和生成器模板，而不是添加固定业务字符串。字段类型可从 resource-id、content-desc、hint 或未来的 inputType 能力推断；无法可靠判断时使用当前通用文本语料。
