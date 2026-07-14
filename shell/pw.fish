function pw
    set -l dir (command pw $argv)
    if test -n "$dir"
        cd -- $dir
    end
end
