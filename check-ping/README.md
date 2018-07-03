# check-ping

## Description

Check ping Response.

## Setting

```
[plugin.checks.icmp]
command = "/path/to/check-ping -H 192.168.0.100 -w "800, 20%" -c "1000, 40%" -p 5 -t 10
```

## Options

```
-w, --warning=N, N%      Exit with WARNING status if RTA less than N (ms) or N% of packet loss
-c, --critical=N, N%     Exit with CRITICAL status if less than N units or N% of disk are free
-H, --host=Host          Host name or IP Address to send ping
-p, --packets=Packets    Packet counts to send
-t, --timeout=Timeout    Timeout (sec)
```
