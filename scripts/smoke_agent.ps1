# Agent-end smoke test (topic 10). ASCII only.
# Usage: powershell -ExecutionPolicy Bypass -File scripts\smoke_agent.ps1
$ErrorActionPreference = "Continue"
$base = "http://localhost:8080"
$agentTok = "agent-token-1"

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

Write-Output "== setup: user4 has only 2 events =="
$src = @{"X-Source-Token" = "teach-events-token"}
Post "$base/api/v1/events" $src '{"event_id":"u4-a1","user_id":4,"article_id":21,"occur_at":"2026-11-11T09:00:00+08:00"}' | Out-Null
Post "$base/api/v1/events" $src '{"event_id":"u4-a2","user_id":4,"article_id":22,"occur_at":"2026-11-11T09:05:00+08:00"}' | Out-Null

Write-Output "== user4 submits appeal =="
Post "$base/api/v1/users/appeals" @{"Authorization" = "Bearer user-token-4"} '{"date":"2026-11-11","content":"third article reward missing"}'

Write-Output "== agent lists open appeals =="
$list = GetReq "$base/api/v1/agent/appeals?status=0" $agentTok
$aid = ($list.appeals | Where-Object { $_.user_id -eq 4 } | Select-Object -First 1).id
Write-Output ("APPEAL_ID=" + $aid)

Write-Output "== agent appeal detail =="
GetReq "$base/api/v1/agent/appeals/$aid" $agentTok | Out-Null

Write-Output "== agent compensates third article =="
Post "$base/api/v1/agent/appeals/$aid/compensate" @{"Authorization" = "Bearer $agentTok"} '{"event_id":"u4-a3","article_id":23,"occur_at":"2026-11-11T09:15:00+08:00"}'

Write-Output "== replay original event must be idempotent (no double reward) =="
Post "$base/api/v1/events" $src '{"event_id":"u4-a3","user_id":4,"article_id":23,"occur_at":"2026-11-11T09:15:00+08:00"}'

Write-Output "== compensate again on resolved appeal must be 409 =="
Post "$base/api/v1/agent/appeals/$aid/compensate" @{"Authorization" = "Bearer $agentTok"} '{"event_id":"u4-x","article_id":24,"occur_at":"2026-11-11T09:20:00+08:00"}'

Write-Output "== reject flow: user5 appeal =="
Post "$base/api/v1/users/appeals" @{"Authorization" = "Bearer user-token-5"} '{"date":"2026-11-11","content":"no reward seen"}'
$list5 = GetReq "$base/api/v1/agent/appeals?status=0" $agentTok
$aid5 = ($list5.appeals | Where-Object { $_.user_id -eq 5 } | Select-Object -First 1).id
Write-Output ("APPEAL5_ID=" + $aid5)
Post "$base/api/v1/agent/appeals/$aid5/reject" @{"Authorization" = "Bearer $agentTok"} '{"conclusion":"only 2 valid articles, below threshold"}'
Write-Output "== second reject must be 409 =="
Post "$base/api/v1/agent/appeals/$aid5/reject" @{"Authorization" = "Bearer $agentTok"} '{"conclusion":"duplicate reject should fail"}'

Write-Output "== user5 points must stay 0 =="
GetReq "$base/api/v1/users/points" "user-token-5" | Out-Null
