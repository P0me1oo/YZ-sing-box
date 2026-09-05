package route

import (
	"context"
	"os"
	"reflect"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/option"
	R "github.com/sagernet/sing-box/route/rule"
	E "github.com/sagernet/sing/common/exceptions"
	"github.com/sagernet/sing/common/task"
	"github.com/sagernet/sing/service"
)

var _ adapter.RuleUpdater = (*Router)(nil)

// UpdateRules 在新规则完成解析和启动后统一替换，失败不推进运行状态。
func (r *Router) UpdateRules(rules []option.Rule, ruleSets []option.RuleSet) error {
	r.reloadAccess.Lock()
	defer r.reloadAccess.Unlock()
	if r.closed {
		return os.ErrClosed
	}
	setsChanged := !reflect.DeepEqual(ruleSets, r.ruleSetOptions)
	if !setsChanged && reflect.DeepEqual(rules, r.ruleOptions) {
		return nil
	}

	newRuleSets, newRuleSetMap := r.ruleSets, r.ruleSetMap
	if setsChanged {
		newRuleSets = make([]adapter.RuleSet, 0, len(ruleSets))
		newRuleSetMap = make(map[string]adapter.RuleSet)
	}
	var newRules []adapter.Rule
	committed := false
	defer func() {
		if committed {
			return
		}
		for _, rule := range newRules {
			_ = rule.Close()
		}
		if setsChanged {
			for _, ruleSet := range newRuleSets {
				_ = ruleSet.Close()
			}
		}
	}()

	// 新规则必须引用本次待提交的规则集，不能绑定到同名旧规则集。
	ctx := service.ExtendContext(r.ctx)
	service.MustRegister[adapter.Router](ctx, &updateRuleSetRouter{Router: r, ruleSets: newRuleSetMap})
	if setsChanged {
		for i, options := range ruleSets {
			for _, tag := range options.Tag {
				if _, exists := newRuleSetMap[tag]; exists {
					return E.New("duplicate rule-set tag: ", tag)
				}
				ruleSet, err := R.NewRuleSet(ctx, r.logger, tag, options)
				if err != nil {
					return E.Cause(err, "parse rule-set[", i, "]")
				}
				newRuleSets = append(newRuleSets, ruleSet)
				newRuleSetMap[tag] = ruleSet
			}
		}
	}
	for i, options := range rules {
		if err := R.ValidateNoNestedRuleActions(options); err != nil {
			return E.Cause(err, "parse rule[", i, "]")
		}
		rule, err := R.NewRule(ctx, r.logger, options, false)
		if err != nil {
			return E.Cause(err, "parse rule[", i, "]")
		}
		newRules = append(newRules, rule)
	}

	if r.started {
		if setsChanged && len(newRuleSets) > 0 {
			startContext := adapter.NewHTTPStartContext()
			defer startContext.Close()
			var group task.Group
			for i, ruleSet := range newRuleSets {
				group.Append0(func(ctx context.Context) error {
					if err := ruleSet.StartContext(ctx, startContext); err != nil {
						return E.Cause(err, "initialize rule-set[", i, "]")
					}
					return nil
				})
			}
			group.Concurrency(5)
			group.FastFail()
			if err := group.Run(ctx); err != nil {
				return err
			}
		}
		for i, rule := range newRules {
			if err := rule.Start(); err != nil {
				return E.Cause(err, "initialize rule[", i, "]")
			}
		}
	}

	// 等待正在进行的路由选择结束；已经建立的连接继续使用原来的出站。
	r.rulesAccess.Lock()
	defer r.rulesAccess.Unlock()
	oldRules, oldRuleSets, oldUpdater := r.rules, r.ruleSets, r.ruleSetUpdater
	r.rules = newRules
	r.ruleSetAccess.Lock()
	r.ruleSets, r.ruleSetMap = newRuleSets, newRuleSetMap
	r.ruleSetAccess.Unlock()
	r.ruleOptions, r.ruleSetOptions = rules, ruleSets
	committed = true
	if r.started && setsChanged {
		if oldUpdater != nil {
			_ = oldUpdater.Close()
		}
		r.ruleSetUpdater = R.NewRuleSetUpdater(r.ctx, newRuleSets)
		if r.ruleSetUpdater != nil {
			r.ruleSetUpdater.Start()
		}
		r.network.Initialize(newRuleSets)
	}
	for _, rule := range oldRules {
		_ = rule.Close()
	}
	if setsChanged {
		for _, ruleSet := range oldRuleSets {
			_ = ruleSet.Close()
		}
		if r.started {
			for _, ruleSet := range newRuleSets {
				ruleSet.Cleanup()
			}
		}
	}
	return nil
}

type updateRuleSetRouter struct {
	adapter.Router
	ruleSets map[string]adapter.RuleSet
}

func (r *updateRuleSetRouter) RuleSet(tag string) (adapter.RuleSet, bool) {
	ruleSet, loaded := r.ruleSets[tag]
	return ruleSet, loaded
}
