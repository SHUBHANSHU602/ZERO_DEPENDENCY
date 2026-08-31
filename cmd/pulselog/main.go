package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"pulselog"
)

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "pulselog: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("pulselog", flag.ContinueOnError)
	flags.SetOutput(stderr)
	dir := flags.String("dir", "./data", "database directory")
	flags.Usage = func() { printUsage(stderr) }
	// Keep the package-level usage hook consistent for callers invoking help.
	flag.Usage = flags.Usage
	if err := flags.Parse(args); err != nil {
		return err
	}

	commandArgs := flags.Args()
	if len(commandArgs) == 0 {
		flags.Usage()
		return errors.New("command required")
	}
	command := commandArgs[0]
	commandArgs = commandArgs[1:]

	switch command {
	case "put":
		if len(commandArgs) < 1 || len(commandArgs) > 2 {
			return errors.New("usage: pulselog put <key> [value]")
		}
		value := []byte(nil)
		if len(commandArgs) == 2 {
			value = []byte(commandArgs[1])
		} else {
			var err error
			value, err = io.ReadAll(stdin)
			if err != nil {
				return fmt.Errorf("read value from stdin: %w", err)
			}
			value = bytes.TrimSuffix(value, []byte("\n"))
			value = bytes.TrimSuffix(value, []byte("\r"))
		}
		return withDB(*dir, func(db *pulselog.DB) error {
			return db.Put(commandArgs[0], value)
		})

	case "get":
		if len(commandArgs) != 1 {
			return errors.New("usage: pulselog get <key>")
		}
		key := commandArgs[0]
		return withDB(*dir, func(db *pulselog.DB) error {
			value, err := db.Get(key)
			if errors.Is(err, pulselog.ErrNotFound) {
				return fmt.Errorf("key %q not found", key)
			}
			if err != nil {
				return err
			}
			if _, err := stdout.Write(value); err != nil {
				return err
			}
			if len(value) == 0 || value[len(value)-1] != '\n' {
				_, err = fmt.Fprintln(stdout)
			}
			return err
		})

	case "query":
		queryFlags := flag.NewFlagSet("query", flag.ContinueOnError)
		queryFlags.SetOutput(stderr)
		fromText := queryFlags.String("from", "", "inclusive RFC3339 start time")
		toText := queryFlags.String("to", "", "inclusive RFC3339 end time")
		if err := queryFlags.Parse(commandArgs); err != nil {
			return err
		}
		if queryFlags.NArg() != 0 || *fromText == "" || *toText == "" {
			return errors.New("usage: pulselog query --from <RFC3339> --to <RFC3339>")
		}
		from, err := time.Parse(time.RFC3339, *fromText)
		if err != nil {
			return fmt.Errorf("invalid --from timestamp: %w", err)
		}
		to, err := time.Parse(time.RFC3339, *toText)
		if err != nil {
			return fmt.Errorf("invalid --to timestamp: %w", err)
		}
		return withDB(*dir, func(db *pulselog.DB) error {
			records, err := db.RangeQuery(from, to)
			if err != nil {
				return err
			}
			for _, record := range records {
				if _, err := fmt.Fprintf(stdout, "%s\t%s\n", record.Key, record.Value); err != nil {
					return err
				}
			}
			return nil
		})

	case "delete":
		if len(commandArgs) != 1 {
			return errors.New("usage: pulselog delete <key>")
		}
		return withDB(*dir, func(db *pulselog.DB) error {
			return db.Delete(commandArgs[0])
		})

	case "compact":
		if len(commandArgs) != 0 {
			return errors.New("usage: pulselog compact")
		}
		return withDB(*dir, func(db *pulselog.DB) error { return db.Compact() })

	default:
		flags.Usage()
		return fmt.Errorf("unknown command %q", command)
	}
}

func withDB(dir string, operation func(*pulselog.DB) error) error {
	db, err := pulselog.Open(dir)
	if err != nil {
		return err
	}
	if err := operation(db); err != nil {
		return errors.Join(err, db.Close())
	}
	return db.Close()
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage: pulselog [--dir <path>] <command> [arguments]")
	fmt.Fprintln(w, "Commands:")
	fmt.Fprintln(w, "  put <key> [value]                         store a value (stdin if omitted)")
	fmt.Fprintln(w, "  get <key>                                 print a value")
	fmt.Fprintln(w, "  query --from <RFC3339> --to <RFC3339>     list records in a time range")
	fmt.Fprintln(w, "  delete <key>                              delete a value")
	fmt.Fprintln(w, "  compact                                   reclaim dead WAL data")
}
