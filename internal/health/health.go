package health

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
)

func Health(ctx context.Context, c *app.RequestContext) {
	c.JSON(200, map[string]interface{}{
		"status": "ok",
	})
}
