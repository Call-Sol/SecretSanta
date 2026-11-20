package draws

import (
	"crypto/rand"
	"io"
	"testing"
)

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 0
	}
	return len(p), nil
}

func TestSingleCycleIsOneLoop(t *testing.T) {
	edges, err := SingleCycle([]int64{10, 20, 30}, zeroReader{})
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateCycle(edges); err != nil {
		t.Fatal(err)
	}
	got := map[int64]int64{}
	for _, edge := range edges {
		got[edge.GiverID] = edge.ReceiverID
	}
	if got[20] != 30 || got[30] != 10 || got[10] != 20 {
		t.Fatalf("edges %+v", got)
	}
}

func TestSingleCycleRejectsShortAndDuplicateLists(t *testing.T) {
	if _, err := SingleCycle([]int64{1, 2}, rand.Reader); err != ErrTooFewParticipants {
		t.Fatalf("short %v", err)
	}
	if _, err := SingleCycle([]int64{1, 1, 2}, rand.Reader); err != ErrDuplicateParticipant {
		t.Fatalf("duplicate %v", err)
	}
}

func TestRandomCyclesStayValid(t *testing.T) {
	for n := 3; n <= 6; n++ {
		ids := make([]int64, n)
		for i := range ids {
			ids[i] = int64(i + 1)
		}
		for try := 0; try < 20; try++ {
			edges, err := SingleCycle(ids, rand.Reader)
			if err != nil {
				t.Fatal(err)
			}
			if err := ValidateCycle(edges); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestValidateCycleRejectsABrokenLoop(t *testing.T) {
	err := ValidateCycle([]Edge{{1, 2}, {2, 1}, {3, 4}})
	if err != ErrInvalidCycle {
		t.Fatalf("got %v", err)
	}
	if err := ValidateCycle([]Edge{{1, 1}, {2, 3}, {3, 2}}); err != ErrInvalidCycle {
		t.Fatalf("self %v", err)
	}
}

func TestIntnUsesTheReader(t *testing.T) {
	n, err := intn(zeroReader{}, 4)
	if err != nil || n != 0 {
		t.Fatalf("n %d err %v", n, err)
	}
	var exhausted io.Reader = eofReader{}
	if _, err := intn(exhausted, 4); err == nil {
		t.Fatal("expected a read error")
	}
}

type eofReader struct{}

func (eofReader) Read([]byte) (int, error) { return 0, io.EOF }
