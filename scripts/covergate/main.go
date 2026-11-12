// 覆盖率门禁：合并多包 profile（同一代码块多份计数求和），
// 校验「自编生产代码整体 ≥85%」且「每个核心业务包 ≥85%」。
// 用法：go test -race -coverpkg=./... -coverprofile=coverage.out ./... && go run ./scripts/covergate coverage.out
package main

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

// 不计入"自编生产代码"的包：进程入口、测试基建与本仓自带的质量工具。
var excluded = map[string]bool{
	"cmd/server":        true,
	"cmd/readingsvc":    true,
	"internal/testutil": true,
	"scripts/covergate": true,
}

// core 核心业务包（要求各自 ≥85%）。
var core = []string{
	"internal/service",
	"internal/store",
	"internal/server",
	"internal/server/handler",
	"internal/readingsvc",
}

type block struct {
	stmts int
	count int
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "用法: go run ./scripts/covergate <profile>")
		os.Exit(2)
	}
	blocks, err := load(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "读取 profile 失败: %v\n", err)
		os.Exit(1)
	}

	total, uncovered := 0, 0
	pkgTotal := map[string]int{}
	pkgUncov := map[string]int{}
	for id, b := range blocks {
		file := strings.SplitN(id, ":", 2)[0]
		file = strings.TrimPrefix(file, "reading-activity/")
		pkg := trimFile(file)
		if excluded[pkg] {
			continue
		}
		total += b.stmts
		pkgTotal[pkg] += b.stmts
		if b.count == 0 {
			uncovered += b.stmts
			pkgUncov[pkg] += b.stmts
		}
	}
	if total == 0 {
		fmt.Fprintln(os.Stderr, "没有可统计的语句")
		os.Exit(1)
	}

	pct := func(u, t int) float64 { return 100 * (1 - float64(u)/float64(t)) }

	keys := make([]string, 0, len(pkgTotal))
	for k := range pkgTotal {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	fail := false
	for _, k := range keys {
		p := pct(pkgUncov[k], pkgTotal[k])
		mark := ""
		for _, c := range core {
			if k == c {
				mark = "（核心包）"
				if p < 85 {
					fail = true
				}
			}
		}
		fmt.Printf("%-26s 语句 %4d  未覆盖 %3d  覆盖率 %5.1f%%%s\n", k, pkgTotal[k], pkgUncov[k], p, mark)
	}
	overall := pct(uncovered, total)
	fmt.Printf("合计（不含 cmd 与 testutil） 语句 %d 未覆盖 %d 覆盖率 %5.1f%%\n", total, uncovered, overall)
	if overall < 85 {
		fail = true
	}
	if fail {
		fmt.Println("门禁结果: 不通过（要求总覆盖 ≥85% 且核心包各自 ≥85%）")
		os.Exit(1)
	}
	fmt.Println("门禁结果: 通过")
}

func trimFile(file string) string {
	i := strings.LastIndex(file, "/")
	if i < 0 {
		return file
	}
	return file[:i]
}

func load(path string) (map[string]*block, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := map[string]*block{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "mode:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 3 {
			continue
		}
		stmts, err1 := strconv.Atoi(fields[1])
		count, err2 := strconv.Atoi(fields[2])
		if err1 != nil || err2 != nil {
			continue
		}
		b := out[fields[0]]
		if b == nil {
			b = &block{}
			out[fields[0]] = b
		}
		b.stmts = stmts
		b.count += count
	}
	return out, sc.Err()
}
