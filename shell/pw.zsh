pw() {
  local dir
  dir="$(command pw "$@")" && [ -n "$dir" ] && cd -- "$dir"
}
