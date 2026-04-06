#!/usr/bin/env bash
set -euo pipefail

# This fixture is for trying to trigger compaction overflow in the summary step,
# not just ordinary compaction.
#
# Usage:
#   bash testdata/compaction_summary_overflow_commands.sh
#
# Optional tuning:
#   TURN_COUNT=18 BLOCK_REPEATS=180 REPLY_STYLE=short bash testdata/compaction_summary_overflow_commands.sh
#
# Notes:
# - Each turn is coherent and asks the assistant to reply tersely.
# - The large payload sits mostly in user turns so the compaction summary input
#   grows quickly.
# - If you need to force an actual summary-window overflow, this fixture is most
#   effective when run against a smaller context window than the normal runtime
#   model window.

TURN_COUNT="${TURN_COUNT:-18}"
BLOCK_REPEATS="${BLOCK_REPEATS:-180}"
REPLY_STYLE="${REPLY_STYLE:-short}"

declare -a FOCUS_AREAS=(
  "trip lifecycle state machine"
  "proof-of-delivery artifact ingestion"
  "dispatch exception ownership"
  "driver offline upload recovery"
  "finance advance reconciliation"
  "customer invoice packet assembly"
  "audit trail event model"
  "incident escalation timeline"
  "route deviation explanation model"
  "maintenance hold policy"
  "fuel fraud review workflow"
  "cross-dock balancing read model"
  "role-based access boundaries"
  "operator remediation hints"
  "customer-facing dispute timeline"
  "support handoff checklist"
  "delayed evidence reconciliation"
  "pilot rollout risk review"
)

reply_constraint() {
  case "$REPLY_STYLE" in
    short)
      printf '%s\n' "Reply in exactly one short paragraph under 80 words. Preserve continuity but do not restate the appendix."
      ;;
    terse)
      printf '%s\n' "Reply in no more than 45 words. Preserve continuity but do not restate the appendix."
      ;;
    *)
      printf '%s\n' "Reply in one compact paragraph. Preserve continuity but do not restate the appendix."
      ;;
  esac
}

appendix_block() {
  local turn="$1"
  local focus="$2"
  local i

  for ((i = 1; i <= BLOCK_REPEATS; i++)); do
    printf 'Appendix segment %03d for turn %02d. Focus area: %s. ' "$i" "$turn" "$focus"
    printf '%s' "Northstar Freight Cloud is piloting across Lagos, Ibadan, Abeokuta, Benin City, and Port Harcourt. "
    printf '%s' "The platform coordinates shipments, trips, stops, route plans, proof artifacts, incident reports, fuel events, maintenance holds, invoice packets, and audit events. "
    printf '%s' "Every mutable object carries created-by, last-updated-by, source-channel, and clock-quality metadata. "
    printf '%s' "Dispatch cells own Apapa pickups, inland cross-dock balancing, eastbound consolidation, and river-port disruption handling. "
    printf '%s' "Operational priorities are reducing empty miles, shortening time-to-reassign after truck failure, improving proof acceptance, and making finance disputes explainable. "
    printf '%s' "Drivers face intermittent connectivity, degraded location precision, delayed receipt capture, handset switching, and resumable upload needs. "
    printf '%s' "Finance needs a chain from approved advance to driver acknowledgment, evidence capture, supervisor validation, billing treatment, and customer packet generation. "
    printf '%s' "The system must preserve why a trip changed, who changed it, which downstream jobs consumed it, and which operators or customers were affected. "
    printf '%s' "Good answers in this thread should keep naming roles, states, failure modes, evidence capture rules, and operator escape hatches for %s. " "$focus"
    printf '%s\n' "This repeated appendix exists to stress long-session memory, compaction, and summary-window behavior while staying semantically coherent."
  done
}

build_message() {
  local turn="$1"
  local focus="$2"

  cat <<EOF
Message $(printf '%02d' "$turn") of ${TURN_COUNT}. Continue the same Northstar Freight Cloud design conversation.
Primary topic for this turn: ${focus}.

Assume all prior decisions still hold unless explicitly contradicted here. Refine the operating model, data model, state transitions, and failure handling for ${focus}. Keep the answer implementation-aware and grounded in Go services, gRPC contracts, Postgres persistence, Redis coordination, S3-compatible artifact storage, and a minimal browser UI. Mention the user roles involved, the events captured, the derived views needed, and the operational tradeoffs that matter during the six-month pilot.

Standing context:
- Pilot geography: Lagos, Ibadan, Abeokuta, Benin City, and Port Harcourt.
- Core users: fleet coordinators, dispatchers, drivers, and finance operators.
- Operational requirements: offline-first behavior, tamper-evident auditability, explicit retry state, operator-visible remediation hints, and explainable timelines.
- Product principles: capture evidence first, classify second; append-only event capture with derived read models; human overrides require actor identity and reason codes.

$(reply_constraint)

Large continuity appendix:
$(appendix_block "$turn" "$focus")
EOF
}

run_turn() {
  local turn="$1"
  local focus="$2"
  local message

  message="$(build_message "$turn" "$focus")"
  hand "$message"
}

main() {
  local index
  local focus_index

  for ((index = 1; index <= TURN_COUNT; index++)); do
    focus_index=$(( (index - 1) % ${#FOCUS_AREAS[@]} ))
    run_turn "$index" "${FOCUS_AREAS[$focus_index]}"
  done
}

main "$@"
