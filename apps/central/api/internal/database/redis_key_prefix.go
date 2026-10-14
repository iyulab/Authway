package database

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/redis/go-redis/v9"
)

var keyPrefixName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,31}$`)

// NormalizeKeyPrefix turns the configured name of a deployment's share of a
// Redis into the prefix its keys carry: the name and a colon. No name is no
// prefix. The name is held to letters, digits, '.', '_' and '-' so that it
// cannot be mistaken for part of a key or for a pattern.
func NormalizeKeyPrefix(configured string) (string, error) {
	name := strings.TrimSuffix(strings.TrimSpace(configured), ":")
	if name == "" {
		if strings.TrimSpace(configured) != "" {
			return "", fmt.Errorf("redis key prefix %q names nothing", configured)
		}
		return "", nil
	}
	if !keyPrefixName.MatchString(name) {
		return "", fmt.Errorf("redis key prefix %q: use up to 32 letters, digits, '.', '_' or '-', starting with a letter or digit", configured)
	}
	return name + ":", nil
}

// UseKeyPrefix makes every key the client names carry prefix. Deployments
// that share one Redis are kept apart this way — like a database index, it is
// a property of the connection, so what uses the client names its keys as if
// the Redis were its own.
//
// It rewrites the keys of the commands listed below and refuses any other
// command that could name a key: one it does not know would write outside
// the prefix, or answer with the keys of every deployment. A command that is
// needed and missing here is added to the list, not worked around.
//
// On a cluster client the keys are rewritten before the command is routed,
// so it goes to the node that holds the prefixed key.
func UseKeyPrefix(client redis.UniversalClient, prefix string) {
	if prefix == "" {
		return
	}
	client.AddHook(keyPrefixHook{prefix: prefix})
}

type keyPrefixHook struct{ prefix string }

func (h keyPrefixHook) DialHook(next redis.DialHook) redis.DialHook { return next }

func (h keyPrefixHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		if err := h.apply(cmd); err != nil {
			cmd.SetErr(err)
			return err
		}
		return next(ctx, cmd)
	}
}

func (h keyPrefixHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		for _, cmd := range cmds {
			if err := h.apply(cmd); err != nil {
				for _, c := range cmds {
					c.SetErr(err)
				}
				return err
			}
		}
		return next(ctx, cmds)
	}
}

// How a command names its keys.
type keyShape int

const (
	noKeys     keyShape = iota // names none
	firstKey                   // the first argument
	everyKey                   // every argument
	countedKey                 // script, key count, then that many keys
)

var keyShapes = map[string]keyShape{
	// Connection and transaction: no keys.
	"ping": noKeys, "echo": noKeys, "hello": noKeys, "auth": noKeys, "select": noKeys,
	"client": noKeys, "multi": noKeys, "exec": noKeys, "discard": noKeys, "script": noKeys,

	// One key, first.
	"get": firstKey, "set": firstKey, "setnx": firstKey, "setex": firstKey, "getdel": firstKey,
	"incr": firstKey, "incrby": firstKey, "decr": firstKey,
	"expire": firstKey, "pexpire": firstKey, "ttl": firstKey, "pttl": firstKey, "persist": firstKey,
	"hset": firstKey, "hget": firstKey, "hgetall": firstKey, "hdel": firstKey, "hincrby": firstKey,

	// Nothing but keys.
	"del": everyKey, "unlink": everyKey, "exists": everyKey,

	// Scripts name their keys after a count.
	"eval": countedKey, "evalsha": countedKey, "eval_ro": countedKey, "evalsha_ro": countedKey,
}

func (h keyPrefixHook) apply(cmd redis.Cmder) error {
	args := cmd.Args()
	if len(args) == 0 {
		return nil
	}
	name := strings.ToLower(fmt.Sprint(args[0]))
	shape, known := keyShapes[name]
	if !known {
		return fmt.Errorf("redis: %s is not a command the key prefix knows how to place; add it to the key prefix's list", name)
	}
	switch shape {
	case firstKey:
		if len(args) > 1 {
			args[1] = h.prefix + fmt.Sprint(args[1])
		}
	case everyKey:
		for i := 1; i < len(args); i++ {
			args[i] = h.prefix + fmt.Sprint(args[i])
		}
	case countedKey:
		if len(args) < 3 {
			return nil
		}
		count, err := strconv.Atoi(fmt.Sprint(args[2]))
		if err != nil || count < 0 || 3+count > len(args) {
			return fmt.Errorf("redis: %s with a key count the key prefix cannot read", name)
		}
		for i := 3; i < 3+count; i++ {
			args[i] = h.prefix + fmt.Sprint(args[i])
		}
	}
	return nil
}
