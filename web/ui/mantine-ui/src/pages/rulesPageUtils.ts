
import { KVSearch } from "@nexucis/kvsearch";
import { Rule, RuleGroup, RulesResult } from "../api/responseTypes/rules";

export type RulesPageData = {
    groups: (RuleGroup & { prefilterRulesCount: number })[];
};

// We need to type the augmented rule for KVSearch
const kvSearch = new KVSearch<Rule & { groupName: string }>({
    shouldSort: true,
    indexedKeys: [
        "name",
        "groupName",
        "query",
        "labels",
        ["labels", /.*/],
    ],
});

export const buildRulesPageData = (
    data: RulesResult,
    search: string,
    healthFilter: (string | null)[]
): RulesPageData => {
    const groups = data.groups.map((group) => {
        // Augment rules with groupName so we can search by group
        const augmentedRules = group.rules.map((r) => ({
            ...r,
            groupName: group.name,
        }));

        const filteredRules = (
            search === ""
                ? augmentedRules
                : kvSearch.filter(search, augmentedRules).map((value) => value.original)
        ).filter(
            (r) => healthFilter.length === 0 || healthFilter.includes(r.health)
        );

        // Map back to original Rule type
        const rules = filteredRules.map((r) => {
            // eslint-disable-next-line @typescript-eslint/no-unused-vars
            const { groupName, ...rest } = r;
            return rest as Rule;
        });

        return {
            ...group,
            prefilterRulesCount: group.rules.length,
            rules,
        };
    });

    return { groups };
};
