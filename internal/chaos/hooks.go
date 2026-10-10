package chaos

import (
	"context"
	"net"

	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"

	"github.com/Ayush1388/auctionEngine/internal/telemetry"
)

// RedisHook fails every Redis dial, command and pipeline while the Redis
// fault is on. Add it once with client.AddHook(chaos.RedisHook{}).
type RedisHook struct{}

func (RedisHook) DialHook(next redis.DialHook) redis.DialHook {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		if err := Fail(Redis); err != nil {
			return nil, err
		}
		return next(ctx, network, addr)
	}
}

func (RedisHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		if err := Fail(Redis); err != nil {
			cmd.SetErr(err)
			return err
		}
		return next(ctx, cmd)
	}
}

func (RedisHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		if err := Fail(Redis); err != nil {
			for _, c := range cmds {
				c.SetErr(err)
			}
			return err
		}
		return next(ctx, cmds)
	}
}

// SlowTracer delays every PostgreSQL statement while PostgresSlow is on and
// otherwise behaves exactly like the telemetry tracer it embeds.
type SlowTracer struct{ telemetry.PGXTracer }

func (t SlowTracer) TraceQueryStart(ctx context.Context, c *pgx.Conn, d pgx.TraceQueryStartData) context.Context {
	Delay(ctx)
	return t.PGXTracer.TraceQueryStart(ctx, c, d)
}

// Ping wraps a readiness probe so it fails while f is broken.
func Ping(f Fault, next func(context.Context) error) func(context.Context) error {
	return func(ctx context.Context) error {
		if err := Fail(f); err != nil {
			return err
		}
		return next(ctx)
	}
}
