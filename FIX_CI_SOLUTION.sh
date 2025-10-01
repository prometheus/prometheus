#!/bin/bash
# CI 測試修復最佳解決方案腳本
# 日期: 2025年10月1日
# 用途: 修復 PR #17251 的 CI 測試失敗

set -e  # 遇到錯誤立即停止

echo "🔧 開始執行 CI 修復程序..."
echo "================================"

# 顏色定義
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# 步驟 1: 備份當前工作
echo -e "${YELLOW}步驟 1: 建立備份分支${NC}"
git branch -D coverage-backup 2>/dev/null || true
git checkout -b coverage-backup
echo -e "${GREEN}✓ 備份完成: coverage-backup${NC}"

# 切回原分支
git checkout feature/promtool-coverage-reporting

# 步驟 2: 儲存本地變更
echo -e "${YELLOW}步驟 2: 儲存本地變更${NC}"
git stash push -m "CI fix temporary stash"
echo -e "${GREEN}✓ 本地變更已儲存${NC}"

# 步驟 3: 同步 upstream
echo -e "${YELLOW}步驟 3: 同步 upstream/main${NC}"
git fetch upstream main
echo -e "${GREEN}✓ upstream/main 已更新${NC}"

# 步驟 4: 嘗試 rebase
echo -e "${YELLOW}步驟 4: 執行智慧 Rebase${NC}"
echo "這可能需要解決衝突..."

# 建立新分支基於最新 upstream
git checkout -b coverage-rebased upstream/main

# Cherry-pick 我們的提交
echo -e "${YELLOW}Cherry-picking 我們的提交...${NC}"

# 獲取我們的提交列表
commits=(
    "95f5b0daa" # feat(promtool): Add comprehensive rule test coverage reporting
    "1cf965ba6" # fix: Remove unused imports from coverage_test.go
    "22c69e0d8" # style: Apply gofmt formatting
    "b738a77e9" # fix: Address linting issues in coverage implementation
    "3b1f9ebc3" # fix: Correct receiver naming issues
    "0e8611d7e" # style: Fix alignment in main.go struct initialization
    "9691be370" # fix: Resolve all linting issues
    "d56b3b676" # style: Fix import ordering and formatting
    "c4ed5e004" # style: Combine parameter declarations in function signature
    "2e6fc9584" # style: Apply gofumpt with extra rules
    "4227225ad" # fix: Correct JSON assertion in coverage test
    "7d753c319" # Fix Windows-specific test error messages in TestCheckConfigSyntax
)

# Cherry-pick 每個提交
for commit in "${commits[@]}"; do
    echo -e "${YELLOW}Applying commit $commit...${NC}"
    if git cherry-pick $commit; then
        echo -e "${GREEN}✓ Successfully applied $commit${NC}"
    else
        echo -e "${RED}⚠ Conflict in $commit - 需要手動解決${NC}"
        echo "解決衝突後，執行:"
        echo "  git add ."
        echo "  git cherry-pick --continue"
        echo ""
        echo "然後繼續執行此腳本"
        exit 1
    fi
done

# 步驟 5: 執行測試
echo -e "${YELLOW}步驟 5: 執行本地測試${NC}"
if go test ./cmd/promtool/... -v; then
    echo -e "${GREEN}✓ 所有測試通過！${NC}"
else
    echo -e "${RED}✗ 測試失敗${NC}"
    echo "請檢查並修復測試錯誤"
    exit 1
fi

# 步驟 6: 更新依賴
echo -e "${YELLOW}步驟 6: 更新 Go 依賴${NC}"
go mod tidy
echo -e "${GREEN}✓ 依賴已更新${NC}"

# 步驟 7: 最終確認
echo ""
echo -e "${GREEN}================================${NC}"
echo -e "${GREEN}🎉 修復程序完成！${NC}"
echo -e "${GREEN}================================${NC}"
echo ""
echo "下一步:"
echo "1. 檢查變更: git diff"
echo "2. 提交變更: git commit -am 'chore: Rebase on latest upstream/main'"
echo "3. 強制推送: git push --force-with-lease origin coverage-rebased"
echo "4. 更新 PR 到新分支"
echo ""
echo "如需回滾:"
echo "  git checkout coverage-backup"