package session

import "github.com/wandxy/hand/pkg/nanoid"

var (
	testSessionA       = nanoid.MustFromSeed(SessionIDPrefix, "project-a", "SessionTestSeedValue123")
	testSessionB       = nanoid.MustFromSeed(SessionIDPrefix, "project-b", "SessionTestSeedValue123")
	testMissingSession = nanoid.MustFromSeed(SessionIDPrefix, "missing", "SessionTestSeedValue123")
	testSessionOne     = nanoid.MustFromSeed(SessionIDPrefix, "session-1", "SessionTestSeedValue123")
	testSessionOlder   = nanoid.MustFromSeed(SessionIDPrefix, "older", "SessionTestSeedValue123")
	testSessionNewer   = nanoid.MustFromSeed(SessionIDPrefix, "newer", "SessionTestSeedValue123")
	testSessionAlpha   = nanoid.MustFromSeed(SessionIDPrefix, "alpha", "SessionTestSeedValue123")
	testSessionZeta    = nanoid.MustFromSeed(SessionIDPrefix, "zeta", "SessionTestSeedValue123")
	testSameTimeA      = nanoid.MustFromSeed(SessionIDPrefix, "same-time-a", "SessionTestSeedValue123")
	testSameTimeB      = nanoid.MustFromSeed(SessionIDPrefix, "same-time-b", "SessionTestSeedValue123")
	testSessionZero    = nanoid.MustFromSeed(SessionIDPrefix, "project-zero", "SessionTestSeedValue123")
)
