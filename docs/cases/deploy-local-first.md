# deploy 本地隔离验收

工作树实现，尚未发布。所有输入来自 `testdata/e2e/repo/skills` 的临时副本；不读取个人配置或真实 targets。

执行报告目录中的 `run-offline.py quickstart python3 specs/010-local-first-entry/evidence/implementation/quickstart.py`，
使用先前 `make build` 生成的本地 `bin/skm`。脚本隔离 HOME、XDG、SKM 插件和 DSH 环境，
显式配置临时 test-a/test-b，不访问网络，不下载二进制或源技能。

覆盖先 init 再部署到当前仓库、--root 指定已初始化仓库、--into 含空格、--no-repo 完整副本、双 target、dry-run 零写入、
已有目录／文件／正常与悬空链接拒绝、force 不绕过新库保护、未选 target 不变及来源删除后可用性。
更细的集合发现、元数据、命名和部分失败断言由对应 Go 服务与 CLI 测试覆盖。

默认模式要求已初始化仓库；缺失时提示先执行 `skm init [DIRECTORY]`，不会隐式创建 `./skm`。
导入已完成但链接失败时，可使用 `skm install NAME --root LIBRARY --target TARGET`。
重复部署默认拒绝已有技能和安装槽，`--force` 允许替换所选技能及安装槽，保留其他仓库内容。
显式 `--into` 仍要求新目录，`--force` 不绕过该限制。
直接模式的重部署重新发现来源，显式 --force 仅覆盖指定槽，不删除上游已经移除的技能。

原始命令、退出码和结果在 `specs/010-local-first-entry/evidence/implementation/` 留存。
