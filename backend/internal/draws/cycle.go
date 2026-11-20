package draws

import (
	"errors"
	"io"
	"math"
)

var (
	// ErrTooFewParticipants means a cycle would be shorter than 3.
	ErrTooFewParticipants = errors.New("too few participants")
	// ErrDuplicateParticipant means an id was repeated or was zero.
	ErrDuplicateParticipant = errors.New("duplicate participant")
	// ErrInvalidCycle means the edges are not one cycle.
	ErrInvalidCycle = errors.New("invalid cycle")
	// ErrAlreadyDrawn means this game already has a draw.
	ErrAlreadyDrawn = errors.New("already drawn")
	// ErrForbidden means the person cannot run the draw.
	ErrForbidden = errors.New("forbidden")
	// ErrNotMember means the person is not an active member of the group.
	ErrNotMember = errors.New("not a member")
	// ErrNotAssigned means this person has no receiver in the active draw.
	ErrNotAssigned = errors.New("not assigned")
	// ErrNotAllowed means the game is not open for a draw.
	ErrNotAllowed = errors.New("draw not allowed")
)

// Edge is one giver to one receiver. IDs are game participant ids.
type Edge struct {
	GiverID    int64
	ReceiverID int64
}

// SingleCycle shuffles ids and links each person to the next, and the last to the first.
func SingleCycle(ids []int64, rng io.Reader) ([]Edge, error) {
	if len(ids) < 3 {
		return nil, ErrTooFewParticipants
	}
	seen := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok || id == 0 {
			return nil, ErrDuplicateParticipant
		}
		seen[id] = struct{}{}
	}
	order := append([]int64(nil), ids...)
	if err := shuffle(order, rng); err != nil {
		return nil, err
	}
	edges := make([]Edge, len(order))
	for i, giver := range order {
		edges[i] = Edge{GiverID: giver, ReceiverID: order[(i+1)%len(order)]}
	}
	return edges, nil
}

// ValidateCycle rejects anything that is not one cycle of length at least 3.
func ValidateCycle(edges []Edge) error {
	if len(edges) < 3 {
		return ErrInvalidCycle
	}
	next := make(map[int64]int64, len(edges))
	incoming := make(map[int64]struct{}, len(edges))
	for _, edge := range edges {
		if edge.GiverID == 0 || edge.ReceiverID == 0 || edge.GiverID == edge.ReceiverID {
			return ErrInvalidCycle
		}
		if _, ok := next[edge.GiverID]; ok {
			return ErrInvalidCycle
		}
		if _, ok := incoming[edge.ReceiverID]; ok {
			return ErrInvalidCycle
		}
		next[edge.GiverID] = edge.ReceiverID
		incoming[edge.ReceiverID] = struct{}{}
	}
	if len(incoming) != len(edges) {
		return ErrInvalidCycle
	}
	start := edges[0].GiverID
	seen := 0
	current := start
	for seen < len(edges) {
		receiver, ok := next[current]
		if !ok {
			return ErrInvalidCycle
		}
		current = receiver
		seen++
		if current == start {
			break
		}
	}
	if seen != len(edges) || current != start {
		return ErrInvalidCycle
	}
	return nil
}

func shuffle(order []int64, rng io.Reader) error {
	for i := len(order) - 1; i > 0; i-- {
		j, err := intn(rng, i+1)
		if err != nil {
			return err
		}
		order[i], order[j] = order[j], order[i]
	}
	return nil
}

func intn(rng io.Reader, n int) (int, error) {
	if n <= 1 {
		return 0, nil
	}
	limit := (math.MaxUint64 / uint64(n)) * uint64(n)
	var buf [8]byte
	for {
		if _, err := io.ReadFull(rng, buf[:]); err != nil {
			return 0, err
		}
		value := uint64(buf[0])<<56 | uint64(buf[1])<<48 | uint64(buf[2])<<40 | uint64(buf[3])<<32 |
			uint64(buf[4])<<24 | uint64(buf[5])<<16 | uint64(buf[6])<<8 | uint64(buf[7])
		if value < limit {
			return int(value % uint64(n)), nil
		}
	}
}
