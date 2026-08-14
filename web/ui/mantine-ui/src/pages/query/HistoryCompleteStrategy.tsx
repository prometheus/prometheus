// Autocompletion strategy that wraps the main one and enriches
// it with past query items.
import {CompleteStrategy} from "@prometheus-io/codemirror-promql";
import {CompletionContext, CompletionResult} from "@codemirror/autocomplete";
import {syntaxTree} from "@codemirror/language";

export class HistoryCompleteStrategy implements CompleteStrategy {
    private complete: CompleteStrategy;
    private queryHistory: string[];
    constructor(complete: CompleteStrategy, queryHistory: string[]) {
        this.complete = complete;
        this.queryHistory = queryHistory;
    }

    promQL(
        context: CompletionContext
    ): Promise<CompletionResult | null> | CompletionResult | null {
        return Promise.resolve(this.complete.promQL(context)).then((res) => {
            const { state, pos } = context;
            const tree = syntaxTree(state).resolve(pos, -1);
            const start = res != null ? res.from : tree.from;

            if (start !== 0) {
                return res;
            }

            const historyItems: CompletionResult = {
                from: start,
                to: pos,
                options: this.queryHistory.map((q) => ({
                    label: q.length < 80 ? q : q.slice(0, 76).concat("..."),
                    detail: "past query",
                    apply: q,
                    info: q.length < 80 ? undefined : q,
                })),
                validFor: /^[a-zA-Z0-9_:]+$/,
            };

            if (res !== null) {
                historyItems.options = historyItems.options.concat(res.options);
            }
            return historyItems;
        });
    }
}