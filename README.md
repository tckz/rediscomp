# rediscomp

Compare content between two redis.  
Currently supports String/Hash only.

## Command line

```text
$ rediscomp [options]

Options:
  -dst value
        host:port of redis. More than once flags can be specified.
  -fetch-count int
        Number of records to scan at once (default 10000)
  -idle-timeout-sec int
        Seconds of pooling idle timeout for redis (default 100)
  -parallel int
        Number of workers (default 8)
  -pool-size int
        Size of pool for redis (default 30)
  -read-timeout-sec int
        Seconds of read timeout for redis (default 5)
  -reverse
        Reverse src and dst
  -scan string
        Match pattern for scan()
  -src value
        host:port of redis. More than once flags can be specified.
  -version
        Show version
```

* One or more --src and --dst must be specified.
* Example: Compare redis on 127.0.0.1:6379 and 192.168.1.11:6379  
  Both of redis are single server.
  ```bash
  $ rediscomp --src 127.0.0.1:6379 --dst 192.168.1.11:6379
  ```
* Example: Src redis is cluster mode
  ```bash
  $ rediscomp --src 127.0.0.1:16379 --src 127.0.0.1:16380 --src 127.0.0.1:16381 --dst 192.168.1.11:6379
  ```

# Build

```bash
$ dep ensure
$ make
```

## Requirement

* Go 1.9
* dep
* git
* make


# My Environment

* macOS High Sierra
* Go 1.9.1
* dep 0.3.1
* git 2.13.5 (Apple Git-94)
* GNU Make 3.81

# License

BSD 2-Clause License

SEE LICENSE
