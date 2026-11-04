---
description: "提交已暂存的变更并推送到远端，不执行 git add"
argument-hint: "[commit message]"
allowed-tools: Bash(git status:*), Bash(git diff:*), Bash(git log:*), Bash(git branch:*), Bash(git commit:*), Bash(git push:*)
---

# 提交暂存区并推送

本指令只处理**已经暂存（staged）**的变更，用户触发本指令即表示授权完成一次 `git commit` 和 `git push`。

## 硬性约束（优先级最高）

- 禁止执行 `git add` / `git stage`，禁止任何把文件加入暂存区的命令或工具操作。
- 禁止 `git commit -a` / `-am` / `--all` / `--amend`，禁止在 `git commit` 后附带文件路径；只允许 `git commit -m "<消息>"`。
- 禁止 `git push --force` / `-f` / `--force-with-lease` / `--set-upstream` / `-u` / `--no-verify`，只执行普通的 `git push`。
- 禁止切换/新建/删除分支、禁止 stash、禁止 reset、禁止修改工作区文件。
- 未暂存（unstaged）和未跟踪（untracked）的文件不属于本次操作范围，必须原样保留。

## 当前仓库上下文

- 当前分支：!`git branch --show-current`
- 状态：!`git status --short`
- 暂存区变更：!`git diff --cached`
- 暂存区统计：!`git diff --cached --stat`
- 最近提交（用于对齐消息风格）：!`git log --oneline -5`

## 执行流程

1. 检查暂存区：`git status --short` 输出的第一列（索引状态）若没有任何变更，说明暂存区为空。此时只回复「暂存区为空，请先自行 `git add`」，然后停止，禁止 commit、禁止 push、禁止替用户暂存。
2. 确定提交消息：
   - 用户参数 `$ARGUMENTS` 非空时，原样作为提交消息。
   - 参数为空时，根据暂存区 diff 生成一条简洁准确的消息，并尽量与 `git log` 中的历史风格保持一致（有 Conventional Commits 风格就沿用）。
3. 执行 `git commit -m "<消息>"`。若失败，报告失败原因并停止，禁止 push。
4. 执行 `git push`。若因没有 upstream 或其他原因失败，如实报告错误并给出建议命令，不要自行补参数重试。
5. 汇报结果：提交哈希、提交消息、推送结果；若还有未暂存的改动，明确指出它们没有被提交、仍保留在工作区。

## 说明

- 不中途向用户二次确认，一次完成 commit + push；遇到异常立即停止并说明原因。
- 不改写历史、不触碰别人的提交、不处理合并冲突，遇到冲突只报告现状。
