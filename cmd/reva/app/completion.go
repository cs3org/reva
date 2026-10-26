// Copyright 2018-2026 CERN
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// In applying this license, CERN does not waive the privileges and immunities
// granted to it by virtue of its status as an Intergovernmental Organization
// or submit itself to any jurisdiction.

package revacmd

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

// completionCommand replaces the completion command of cobra. Its bash
// script also works without the bash-completion package.
func completionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "completion [bash|zsh|fish]",
		Short: "Print a shell completion script",
		Long: "Print a shell completion script.\n\n" +
			"  bash:  source <(reva completion bash)\n" +
			"  zsh:   reva completion zsh > \"${fpath[1]}/_reva\"\n" +
			"  fish:  reva completion fish | source\n\n" +
			"Once it is loaded, TAB completes remote paths, share ids and the other\n" +
			"values that the server knows.",
		Args:      cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		ValidArgs: []string{"bash", "zsh", "fish"},
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			switch args[0] {
			case "bash":
				if _, err := io.WriteString(out, bashCompletionShims); err != nil {
					return err
				}
				return cmd.Root().GenBashCompletionV2(out, true)
			case "zsh":
				return cmd.Root().GenZshCompletion(out)
			case "fish":
				return cmd.Root().GenFishCompletion(out, true)
			}
			return fmt.Errorf("unknown shell %q: use bash, zsh or fish", args[0])
		},
	}
}

// bashCompletionShims define the two functions of the bash-completion package
// that the cobra script calls, only when they are missing. A real
// bash-completion, loaded before or after, always wins.
const bashCompletionShims = `# Stand-ins for the bash-completion package, used only where it is not installed.
if ! declare -F _get_comp_words_by_ref >/dev/null 2>&1; then
_get_comp_words_by_ref()
{
    local exclude="" flag OPTIND=1
    while getopts "n:" flag "$@"; do
        [[ $flag == n ]] && exclude=$OPTARG
    done
    shift $((OPTIND-1))

    # Rejoin the pieces COMP_WORDBREAKS split off at an excluded character,
    # walking COMP_LINE to tell adjacent pieces from separate words.
    local line=${COMP_LINE:0:COMP_POINT} trimmed tok sep excl prevExcl=0 i cw=-1
    local -a out=()
    for ((i = 0; i < ${#COMP_WORDS[@]}; i++)); do
        tok=${COMP_WORDS[i]}
        trimmed=${line#"${line%%[![:space:]]*}"}
        sep=$(( ${#trimmed} != ${#line} ))
        line=$trimmed
        ((${#line} < ${#tok})) && tok=$line
        line=${line:${#tok}}
        excl=0
        [[ -n $tok && -n $exclude && -z ${tok//["$exclude"]/} ]] && excl=1
        if ((i > 0 && !sep && (excl || prevExcl))); then
            out[${#out[@]}-1]+=$tok
        else
            out+=("$tok")
        fi
        prevExcl=$excl
        if ((i == COMP_CWORD)); then
            cw=$((${#out[@]} - 1))
            break
        fi
    done
    ((cw < 0)) && cw=$((${#out[@]} - 1))

    local var
    for var in "$@"; do
        case $var in
        cur) cur=${out[cw]} ;;
        prev) prev=""; ((cw > 0)) && prev=${out[cw-1]} ;;
        words) words=("${out[@]}") ;;
        cword) cword=$cw ;;
        esac
    done
}
fi

if ! declare -F _filedir >/dev/null 2>&1; then
_filedir()
{
    local reply f
    if [[ ${1-} == -d ]]; then
        reply=$(compgen -d -- "$cur")
    elif [[ -n ${1-} ]]; then
        reply=$(shopt -s extglob; compgen -d -- "$cur"; compgen -f -X "!*.@(${1%|})" -- "$cur")
    else
        reply=$(compgen -f -- "$cur")
    fi
    while IFS= read -r f; do
        [[ -n $f ]] && COMPREPLY+=("$f")
    done <<<"$reply"
    compopt -o filenames 2>/dev/null
    return 0
}
fi

`
