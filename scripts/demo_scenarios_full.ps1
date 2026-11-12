# 题目 10 必演场景补充演示：S1 从运营发布开始、S2 跨日迟到/活动结束后窗口、S5 并发与窗口关闭核查。
# 用法：powershell -ExecutionPolicy Bypass -File scripts\demo_scenarios_full.ps1 -Mode s1s5|clock
# 依赖：主服务运行于 :8080；A2 活动由本脚本通过运营接口发布。
param([string]$Mode = "s1s5", [string]$Base = "http://localhost:8080")

$base = $Base
$src = @{"X-Source-Token" = "teach-events-token"}
$agent = @{"Authorization" = "Bearer agent-token-1"}
$bj = [TimeZoneInfo]::ConvertTimeBySystemTimeZoneId([DateTime]::UtcNow, "China Standard Time")

function Post($url, $headers, $body) {
    try {
        $r = Invoke-WebRequest -Uri $url -Method POST -Headers $headers -ContentType "application/json; charset=utf-8" -Body ([Text.Encoding]::UTF8.GetBytes($body)) -UseBasicParsing
        Write-Output ("{0} {1}" -f $r.StatusCode, $r.Content)
        return ($r.Content | ConvertFrom-Json)
    } catch {
        Write-Output ("{0} {1}" -f [int]$_.Exception.Response.StatusCode, $_.ErrorDetails.Message)
        return $null
    }
}
function GetReq($url, $token) {
    try {
        $r = Invoke-WebRequest -Uri $url -Headers @{"Authorization" = "Bearer $token"} -UseBasicParsing
        Write-Output ("{0} {1}" -f $r.StatusCode, $r.Content)
        return ($r.Content | ConvertFrom-Json)
    } catch {
        Write-Output ("{0} {1}" -f [int]$_.Exception.Response.StatusCode, $_.ErrorDetails.Message)
        return $null
    }
}

if ($Mode -eq "s1s5") {
    Write-Output ("== S1: 运营发布活动（当前时钟 " + $bj.ToString("yyyy-MM-dd HH:mm") + "）==")
    Post "$base/api/v1/ops/activity" @{"Authorization" = "Bearer ops-token-1"} '{"name":"双日阅读活动A2","start_at":"2026-11-12T09:00:00+08:00","end_at":"2026-11-14T11:00:00+08:00","daily_threshold":3,"reward_points":30}'

    Write-Output "== S1: 用户2 阅读 3 篇 → 自动发奖 =="
    Post "$base/api/v1/events" $src '{"event_id":"d-s1-1","user_id":2,"article_id":71,"occur_at":"2026-11-12T10:00:00+08:00"}'
    Post "$base/api/v1/events" $src '{"event_id":"d-s1-2","user_id":2,"article_id":72,"occur_at":"2026-11-12T10:05:00+08:00"}'
    Post "$base/api/v1/events" $src '{"event_id":"d-s1-3","user_id":2,"article_id":73,"occur_at":"2026-11-12T10:10:00+08:00"}'
    Write-Output "== S1: 用户2 查流水 =="
    GetReq "$base/api/v1/users/points" "user-token-2" | Out-Null

    Write-Output "== S5: 用户5 先读 2 篇，申诉后自动入账与客服补入并发投递 =="
    Post "$base/api/v1/events" $src '{"event_id":"d-s5-1","user_id":5,"article_id":51,"occur_at":"2026-11-12T10:00:00+08:00"}' | Out-Null
    Post "$base/api/v1/events" $src '{"event_id":"d-s5-2","user_id":5,"article_id":52,"occur_at":"2026-11-12T10:05:00+08:00"}' | Out-Null
    Post "$base/api/v1/users/appeals" @{"Authorization" = "Bearer user-token-5"} '{"date":"2026-11-12","content":"并发场景：第3篇与客服补入同时到达"}'
    $list = GetReq "$base/api/v1/agent/appeals?status=0" "agent-token-1"
    $aid = ($list.appeals | Where-Object { $_.user_id -eq 5 } | Select-Object -First 1).id
    Write-Output ("CONCURRENT_APPEAL_ID=" + $aid)

    # 真并发：两个 curl 进程同时发出（Windows 自带 curl.exe，毫秒级同发）
    $b1 = '{"event_id":"d-s5-auto","user_id":5,"article_id":53,"occur_at":"2026-11-12T10:20:00+08:00"}'
    $b2 = '{"event_id":"d-s5-agent","article_id":54,"occur_at":"2026-11-12T10:21:00+08:00"}'
    $o1 = "$env:TEMP\s5_auto.json"; $o2 = "$env:TEMP\s5_agent.json"
    $f1 = "$env:TEMP\s5_auto_body.json"; $f2 = "$env:TEMP\s5_agent_body.json"
    [IO.File]::WriteAllText($f1, $b1, (New-Object System.Text.UTF8Encoding($false)))
    [IO.File]::WriteAllText($f2, $b2, (New-Object System.Text.UTF8Encoding($false)))
    # body 走 @file，避免引号被命令行解析破坏
    $a1 = "-sS -o ""$o1"" -X POST ""$base/api/v1/events"" -H ""X-Source-Token: teach-events-token"" -H ""Content-Type: application/json"" -d @""$f1"""
    $a2 = "-sS -o ""$o2"" -X POST ""$base/api/v1/agent/appeals/$aid/compensate"" -H ""Authorization: Bearer agent-token-1"" -H ""Content-Type: application/json"" -d @""$f2"""
    $p1 = Start-Process -FilePath "curl.exe" -ArgumentList $a1 -PassThru -WindowStyle Hidden
    $p2 = Start-Process -FilePath "curl.exe" -ArgumentList $a2 -PassThru -WindowStyle Hidden
    $p1.WaitForExit(30000) | Out-Null; $p2.WaitForExit(30000) | Out-Null
    Write-Output ("auto=" + (Get-Content $o1 -Encoding utf8))
    Write-Output ("agent=" + (Get-Content $o2 -Encoding utf8))

    Write-Output "== S1/S5: 结算汇总（window_closed 应为 false）=="
    GetReq "$base/api/v1/ops/summary" "ops-token-1"
}
elseif ($Mode -eq "clock") {
    Write-Output ("== S2/S5: 教学时钟 +50h（活动 11-14 11:00 结束，当前约 11-14 12:30）==")
    Write-Output "== S2: 活动结束后窗口内事件（occur 11-13 22:00，接收 11-14 12:30，迟到 14.5h）=="
    Post "$base/api/v1/events" $src '{"event_id":"d-clk-1","user_id":3,"article_id":91,"occur_at":"2026-11-13T22:00:00+08:00"}'
    Write-Output "== S2: 超窗事件（occur 11-12 09:30，迟到 >50h）=="
    Post "$base/api/v1/events" $src '{"event_id":"d-clk-2","user_id":3,"article_id":92,"occur_at":"2026-11-12T09:30:00+08:00"}'
    Write-Output "== S2: 跨日迟到归发生日（occur 11-13 23:30 → biz_date 11-13）=="
    Post "$base/api/v1/events" $src '{"event_id":"d-clk-3","user_id":3,"article_id":93,"occur_at":"2026-11-13T23:30:00+08:00"}'
    Write-Output "== S5: 窗口关闭后终局核查（window_closed 应为 true）=="
    GetReq "$base/api/v1/ops/summary" "ops-token-1"
}
elseif ($Mode -eq "summary") {
    Write-Output ("== S5: 终局核查（当前时钟 " + $bj.ToString("yyyy-MM-dd HH:mm") + "，应 > 活动结束+24h）==")
    GetReq "$base/api/v1/ops/summary" "ops-token-1"
}
