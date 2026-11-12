# 题目 10 必演场景演示（S1/S2/S3/S4/S5 关键链路）。ASCII only.
# 使用当前真实时间（活动期内），在 reading_activity 演示库留持久化结果。
$ErrorActionPreference = "Continue"
$base = "http://localhost:8080"
$src = @{"X-Source-Token" = "teach-events-token"}
$bj = [TimeZoneInfo]::ConvertTimeBySystemTimeZoneId([DateTime]::UtcNow, "China Standard Time")
$occ = $bj.ToString("yyyy-MM-ddTHH:mm:sszzz")
$today = $bj.ToString("yyyy-MM-dd")
Write-Output ("NOW=$occ")

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

Write-Output "== S2: duplicate article + duplicate event (user6) =="
Post "$base/api/v1/events" $src ('{"event_id":"d-s2-1","user_id":6,"article_id":31,"occur_at":"' + $occ + '"}')
Post "$base/api/v1/events" $src ('{"event_id":"d-s2-1","user_id":6,"article_id":31,"occur_at":"' + $occ + '"}')
Post "$base/api/v1/events" $src ('{"event_id":"d-s2-2","user_id":6,"article_id":31,"occur_at":"' + $occ + '"}')
Post "$base/api/v1/events" $src ('{"event_id":"d-s2-3","user_id":6,"article_id":31,"occur_at":"2026-11-10T09:00:00+08:00"}')

Write-Output "== S3: user4 reads 2 valid articles, 3rd missing -> appeal -> compensate -> reward =="
Post "$base/api/v1/events" $src ('{"event_id":"d-s3-1","user_id":4,"article_id":41,"occur_at":"' + $occ + '"}')
Post "$base/api/v1/events" $src ('{"event_id":"d-s3-2","user_id":4,"article_id":42,"occur_at":"' + $occ + '"}')
Post "$base/api/v1/users/appeals" @{"Authorization" = "Bearer user-token-4"} ('{"date":"' + $today + '","content":"third article trusted record not delivered"}')
$list = GetReq "$base/api/v1/agent/appeals?status=0" "agent-token-1"
$aid = ($list.appeals | Where-Object { $_.user_id -eq 4 } | Select-Object -First 1).id
Write-Output ("APPEAL_ID=" + $aid)
GetReq "$base/api/v1/agent/appeals/$aid" "agent-token-1" | Out-Null
Post "$base/api/v1/agent/appeals/$aid/compensate" @{"Authorization" = "Bearer agent-token-1"} ('{"event_id":"d-s3-3","article_id":43,"occur_at":"' + $occ + '"}')
Write-Output "== S3: replay same event must be idempotent =="
Post "$base/api/v1/events" $src ('{"event_id":"d-s3-3","user_id":4,"article_id":43,"occur_at":"' + $occ + '"}')
Write-Output "== S3: compensate again must be 409 =="
Post "$base/api/v1/agent/appeals/$aid/compensate" @{"Authorization" = "Bearer agent-token-1"} ('{"event_id":"d-s3-x","article_id":44,"occur_at":"' + $occ + '"}')
Write-Output "== S3: user4 balance must be 30 =="
GetReq "$base/api/v1/users/points" "user-token-4"

Write-Output "== S4: user5 has 2 articles only -> appeal -> reject -> points stay 0 =="
Post "$base/api/v1/events" $src ('{"event_id":"d-s4-1","user_id":5,"article_id":51,"occur_at":"' + $occ + '"}')
Post "$base/api/v1/events" $src ('{"event_id":"d-s4-2","user_id":5,"article_id":52,"occur_at":"' + $occ + '"}')
Post "$base/api/v1/users/appeals" @{"Authorization" = "Bearer user-token-5"} ('{"date":"' + $today + '","content":"think reward missing"}')
$list5 = GetReq "$base/api/v1/agent/appeals?status=0" "agent-token-1"
$aid5 = ($list5.appeals | Where-Object { $_.user_id -eq 5 } | Select-Object -First 1).id
Post "$base/api/v1/agent/appeals/$aid5/reject" @{"Authorization" = "Bearer agent-token-1"} '{"conclusion":"only 2 valid articles, below daily threshold 3"}'
Write-Output "== S4: second reject must be 409 =="
Post "$base/api/v1/agent/appeals/$aid5/reject" @{"Authorization" = "Bearer agent-token-1"} '{"conclusion":"duplicate reject should fail"}'
Write-Output "== S4: user5 points must be 0 =="
GetReq "$base/api/v1/users/points" "user-token-5"

Write-Output "== S5: cross-user access must be denied =="
GetReq "$base/api/v1/users/progress?date=$today" "user-token-4"
try { Invoke-WebRequest -Uri "$base/api/v1/agent/appeals?status=0" -Headers @{"Authorization" = "Bearer user-token-4"} -UseBasicParsing | Out-Null } catch { Write-Output ("{0} role-violation denied" -f [int]$_.Exception.Response.StatusCode) }
try { Invoke-WebRequest -Uri "$base/api/v1/events" -Method POST -Headers @{"Authorization" = "Bearer user-token-4"} -ContentType "application/json" -Body "{}" -UseBasicParsing | Out-Null } catch { Write-Output ("{0} events-token denied" -f [int]$_.Exception.Response.StatusCode) }

Write-Output "== S1/S8: ops summary =="
GetReq "$base/api/v1/ops/summary" "ops-token-1"
