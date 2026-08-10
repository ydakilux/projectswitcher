function pw
    set -l out (command pw $argv)
    or return $status
    set -l dir $out[1]
    set -l act $out[2]
    set -l ed $out[3]
    if test -n "$dir"
        cd $dir
    end
    switch "$act"
        case opencode
            opencode
        case editor
            if test -n "$ed"
                $ed .
            end
    end
end
