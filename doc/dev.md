# Development notes

## Directories

1. `core` implements `Location`, which packages up rules functionality
   for a single location.

2. `sys` provides an example location container (`System`), which can
   host multiple locations.

3. `service` provides a clumsy HTTP wrapper around a `sys.System`.

4. `rulesys` is a command that starts a `service`.

5. `cron` defines a generic cron service, an glue for an
   implementation called "Crolt" based on
   [BoltDB](https://github.com/boltdb/bolt), and a simple-minded,
   in-memory cron implementation ("internal").

6. `crolt` is the little cron based on
   [BoltDB](https://github.com/boltdb/bolt).

7. `storage` contains some implementations of the `core.Storage`
   interface.

## Development

If you want to talk to a rules engine via HTTP, you can use `rulesys`.
See the top-level README and `examples/` for examples.

If you want to wrap another API around this rules engine, you'll
program against `sys.System`.  See `service/` and `rulesys/` for
examples.

If you don't need multiple locations or otherwise desire the simplest
thing, program against `core` directly.

