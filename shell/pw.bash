pw() {
  local out dir act ed
  out="$(command pw "$@")" || return $?
  dir="$(printf '%s\n' "$out" | sed -n '1p')"
  act="$(printf '%s\n' "$out" | sed -n '2p')"
  ed="$(printf '%s\n' "$out" | sed -n '3p')"
  [ -n "$dir" ] && cd -- "$dir"
  case "$act" in
    opencode) opencode ;;
    editor)   [ -n "$ed" ] && "$ed" . ;;
  esac
}
