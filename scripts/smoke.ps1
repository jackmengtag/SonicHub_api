﻿﻿﻿﻿﻿﻿﻿﻿﻿param(
    [string]$Base = "http://127.0.0.1:8080",
    [string]$Phone = "13800138000",
    [string]$Code = "1234"
)

$ErrorActionPreference = "Stop"

function Request($Method, $Path, $Body, $Token) {
    $headers = @{}
    if ($Token) { $headers["Authorization"] = "Bearer $Token" }
    $args = @{
        Method      = $Method
        Uri         = "$Base$Path"
        Headers     = $headers
        ContentType = "application/json; charset=utf-8"
        TimeoutSec  = 10
    }
    if ($null -ne $Body) {
        $args["Body"] = ($Body | ConvertTo-Json -Depth 8)
    }
    return Invoke-RestMethod @args
}

function Step($Name) { Write-Output "== $Name" }

Step "health"
$health = Request "GET" "/health" $null $null
Write-Output "  $($health.data.status)"

Step "login"
$login = Request "POST" "/api/v1/auth/login" @{ phone = $Phone; code = $Code } $null
$token = $login.data.access_token
Write-Output "  user=$($login.data.user.display_name) tenant=$($login.data.tenant_id)"

Step "me"
$me = Request "GET" "/api/v1/me" $null $token
Write-Output "  credits=$($me.data.credits.available) works=$($me.data.counts.works) friends=$($me.data.counts.friends)"

Step "feed"
$feed = Request "GET" "/api/v1/feed/recommend?limit=5" $null $token
Write-Output "  items=$($feed.data.items.Count) policy=$($feed.data.policy_version)"

Step "plaza"
$plaza = Request "GET" "/api/v1/plaza?sort=hot&limit=5" $null $null
Write-Output "  items=$($plaza.data.items.Count) first=$($plaza.data.items[0].title)"

Step "search"
$search = Request "GET" "/api/v1/search?q=山海" $null $null
Write-Output "  works=$($search.data.works.Count) artists=$($search.data.artists.Count)"

Step "social"
$follow = Request "POST" "/api/v1/users/u_bailu/follow" @{} $token
Write-Output "  following=$($follow.data.following) friend=$($follow.data.friend)"
$friends = Request "GET" "/api/v1/me/friends" $null $token
Write-Output "  friends=$($friends.data.items.Count)"

Step "quote"
$beforeCredits = Request "GET" "/api/v1/me/credits" $null $token
$quote = Request "POST" "/api/v1/credit/quotes" @{
    styles = @("电子"); duration_seconds = 30; quality = "高清"; stems = $true
} $token
Write-Output "  quote=$($quote.data.quote_id) total=$($quote.data.total)"

Step "generation"
$gen = Request "POST" "/api/v1/generations" @{
    quote_id = $quote.data.quote_id
    prompt = "雨后的城市，路灯把水洼照亮"
    styles = @("电子", "Lo-fi")
    duration_seconds = 30
    vocal = "女声"
    quality = "高清"
    stems = $true
    title = "冒烟测试作品"
    visibility = "public"
    auto_publish = $true
} $token
$genID = $gen.data.id
Write-Output "  generation=$genID status=$($gen.data.status)"

Step "generation worker"
$final = $null
for ($i = 0; $i -lt 30; $i++) {
    Start-Sleep -Seconds 1
    $current = Request "GET" "/api/v1/generations/$genID" $null $token
    if ($current.data.status -eq "succeeded") { $final = $current.data; break }
    if ($current.data.status -eq "failed" -or $current.data.status -eq "cancelled") {
        throw "generation ended with $($current.data.status): $($current.data.error)"
    }
}
if (-not $final) { throw "generation timed out" }
Write-Output "  status=$($final.status) work=$($final.work_id)"

Step "credits after generation"
$afterCredits = Request "GET" "/api/v1/me/credits" $null $token
$spent = [int64]$beforeCredits.data.available - [int64]$afterCredits.data.available
if ($spent -ne [int64]$quote.data.total) { throw "expected spend $($quote.data.total), got $spent" }
if ([int64]$afterCredits.data.reserved -ne 0) { throw "reserved credit should be released/captured" }
$ledger = Request "GET" "/api/v1/me/credits/ledger?limit=10" $null $token
Write-Output "  available=$($afterCredits.data.available) spent=$spent ledger=$($ledger.data.items.Count)"

Step "work"
$work = Request "GET" "/api/v1/works/$($final.work_id)" $null $token
Write-Output "  title=$($work.data.title) audio=$($work.data.audio_url) status=$($work.data.status)"

Step "media"
$media = Invoke-WebRequest -Uri "$Base$($work.data.audio_url)" -Method GET -TimeoutSec 10
Write-Output "  audio http=$($media.StatusCode) bytes=$($media.RawContentLength)"

Step "like and comment"
$like = Request "POST" "/api/v1/works/$($work.data.id)/like" @{} $token
$comment = Request "POST" "/api/v1/works/$($work.data.id)/comments" @{ content = "自动化冒烟测试评论" } $token
Write-Output "  likes=$($like.data.like_count) comment=$($comment.data.id)"

Step "share"
$link = Request "POST" "/api/v1/share-links" @{ work_id = $work.data.id } $token
$landing = Request "GET" "/api/v1/s/$($link.data.code)" $null $null
Write-Output "  code=$($link.data.code) landing=$($landing.data.work.title)"

Step "notifications"
$notes = Request "GET" "/api/v1/notifications?limit=10" $null $token
Write-Output "  items=$($notes.data.items.Count) unread=$($notes.data.unread)"

Step "admin"
$stats = Request "GET" "/api/v1/admin/stats" $null $token
Write-Output "  pending=$($stats.data.pending) reports=$($stats.data.reports) users=$($stats.data.users)"

Write-Output "SMOKE PASS"
