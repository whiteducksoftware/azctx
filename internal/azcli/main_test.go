package azcli_test

import (
	"os"
	"testing"

	"github.com/whiteducksoftware/azctx/internal/testutil"
)

func TestMain(m *testing.M) {
	os.Exit(testutil.RunTests(m))
}
