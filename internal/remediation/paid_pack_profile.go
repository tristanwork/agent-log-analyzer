package remediation

import (
	"encoding/json"
	"sort"

	"github.com/priivacy-ai/agent-log-analyzer/internal/analyzer"
)

type PaidPackProfile struct {
	SchemaVersion   string                 `json:"schema_version"`
	PrivacyBoundary []string               `json:"privacy_boundary"`
	MCP             PaidPackSurfaceProfile `json:"mcp"`
	Skill           PaidPackSurfaceProfile `json:"skill"`
	VettedTools     []PaidPackToolProfile  `json:"vetted_tools"`
	Waiver          PaidPackWaiver         `json:"waiver"`
}

type PaidPackSurfaceProfile struct {
	WarningBand             string   `json:"warning_band,omitempty"`
	ContextTokenBucket      string   `json:"context_token_bucket,omitempty"`
	ContextEfficiencyBucket string   `json:"context_efficiency_bucket,omitempty"`
	UtilizationRatioPct     int      `json:"utilization_ratio_pct,omitempty"`
	ExposureKnown           bool     `json:"exposure_known"`
	KnownConfiguredIDs      []string `json:"known_configured_ids"`
	KnownUsedIDs            []string `json:"known_used_ids"`
	UnknownConfiguredCount  int      `json:"unknown_configured_count,omitempty"`
	UnknownUsedCount        int      `json:"unknown_used_count,omitempty"`
	Guidance                []string `json:"guidance"`
}

type PaidPackToolProfile struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	SourceURL        string   `json:"source_url"`
	Category         string   `json:"category"`
	Reason           string   `json:"reason"`
	SignalIDs        []string `json:"signal_ids"`
	FailureModes     []string `json:"failure_modes"`
	Confidence       string   `json:"confidence"`
	RiskLevel        string   `json:"risk_level"`
	DataMovementRisk string   `json:"data_movement_risk"`
	InstallPolicy    string   `json:"install_policy"`
	InstallSurface   string   `json:"install_surface"`
	RollbackGuidance string   `json:"rollback_guidance,omitempty"`
	WaiverRequired   bool     `json:"waiver_required"`
	VettingNotes     string   `json:"vetting_notes,omitempty"`
}

type PaidPackWaiver struct {
	Required bool     `json:"required"`
	Reasons  []string `json:"reasons,omitempty"`
	Language string   `json:"language"`
}

func GeneratePaidPackProfile(report analyzer.Report) PaidPackProfile {
	tools := paidPackToolProfiles(report)
	waiver := paidPackWaiver(tools)
	return PaidPackProfile{
		SchemaVersion: "2026-05-31-paid-pack-profile",
		PrivacyBoundary: []string{
			"Uses aggregate buckets, allowlisted public IDs, and bounded counts only.",
			"Does not include private MCP names, private skill text, local paths, repo names, prompts, file contents, or raw tool output.",
			"Unknown MCP and skill entries are represented as counts until each ID is allowlisted.",
		},
		MCP:         paidPackMCPProfile(report.Ecosystem.ToolingUtilization.MCP),
		Skill:       paidPackSkillProfile(report.Ecosystem.ToolingUtilization.Skill),
		VettedTools: tools,
		Waiver:      waiver,
	}
}

func paidPackProfileJSON(report analyzer.Report) string {
	body, err := json.MarshalIndent(GeneratePaidPackProfile(report), "", "  ")
	if err != nil {
		return "{}\n"
	}
	return string(body) + "\n"
}

func paidPackMCPProfile(mcp analyzer.MCPUtilization) PaidPackSurfaceProfile {
	guidance := []string{
		"Keep the default MCP surface small; lazy-load project-specific servers only when a task requires live external data.",
	}
	if isAttentionBand(mcp.WarningBand) {
		guidance = append(guidance, "Move nonessential MCP servers behind task-specific setup so their schemas do not load into every session.")
	}
	if mcp.ExposureKnown && mcp.UtilizationRatioPct > 0 && mcp.UtilizationRatioPct < 25 {
		guidance = append(guidance, "Disable or defer rarely used allowlisted MCP servers before adding new MCP surface area.")
	}
	if mcp.UnknownServerCount > 0 || mcp.UniqueUnknownCalledCount > 0 || mcp.UnknownCallCount > 0 {
		guidance = append(guidance, "Audit unknown MCP exposure by count first; publish IDs only after they match the public allowlist.")
	}
	if mcp.ContextTokenBucket != "" {
		guidance = append(guidance, "Use the MCP context-token bucket to decide whether setup belongs in default config or a task-specific profile.")
	}
	return PaidPackSurfaceProfile{
		WarningBand:             mcp.WarningBand,
		ContextTokenBucket:      mcp.ContextTokenBucket,
		ContextEfficiencyBucket: mcp.ContextEfficiencyBucket,
		UtilizationRatioPct:     mcp.UtilizationRatioPct,
		ExposureKnown:           mcp.ExposureKnown,
		KnownConfiguredIDs:      allowlistedIDs("mcp", mcp.KnownServerIDs),
		KnownUsedIDs:            allowlistedIDs("mcp", mcp.UniqueKnownCalledIDs),
		UnknownConfiguredCount:  mcp.UnknownServerCount,
		UnknownUsedCount:        maxInt(mcp.UniqueUnknownCalledCount, mcp.UnknownCallCount),
		Guidance:                guidance,
	}
}

func paidPackSkillProfile(skill analyzer.SkillUtilization) PaidPackSurfaceProfile {
	guidance := []string{
		"Keep always-loaded skills short and move detailed workflow knowledge into scoped skills that load only when relevant.",
	}
	if isAttentionBand(skill.WarningBand) {
		guidance = append(guidance, "Split broad skill profiles by workflow so inactive guidance does not occupy default context.")
	}
	if skill.ExposureKnown && skill.UtilizationRatioPct > 0 && skill.UtilizationRatioPct < 25 {
		guidance = append(guidance, "Demote rarely executed allowlisted skills into explicit commands or task-specific references.")
	}
	if skill.UnknownExposedCount > 0 || skill.UnknownExecutedCount > 0 {
		guidance = append(guidance, "Audit unknown skill exposure by count first; do not print private skill names or skill text into the paid pack.")
	}
	if skill.ContextTokenBucket != "" {
		guidance = append(guidance, "Use the skill context-token bucket to decide what stays always loaded versus what becomes a scoped reference.")
	}
	return PaidPackSurfaceProfile{
		WarningBand:             skill.WarningBand,
		ContextTokenBucket:      skill.ContextTokenBucket,
		ContextEfficiencyBucket: skill.ContextEfficiencyBucket,
		UtilizationRatioPct:     skill.UtilizationRatioPct,
		ExposureKnown:           skill.ExposureKnown,
		KnownConfiguredIDs:      allowlistedIDs("skill", skill.KnownExposedIDs),
		KnownUsedIDs:            allowlistedIDs("skill", skill.KnownExecutedIDs),
		UnknownConfiguredCount:  skill.UnknownExposedCount,
		UnknownUsedCount:        skill.UnknownExecutedCount,
		Guidance:                guidance,
	}
}

func paidPackToolProfiles(report analyzer.Report) []PaidPackToolProfile {
	if report.Recommendation == nil {
		return nil
	}
	var out []PaidPackToolProfile
	seen := map[string]bool{}
	add := func(rec *analyzer.TokenSavingRecommendation) {
		if rec == nil || rec.PrimaryToolID == "" {
			return
		}
		tool, ok := analyzer.GetTool(rec.PrimaryToolID)
		if !ok || !tool.PaidPackAllowed || tool.ResearchOnly || tool.InstallPolicy == analyzer.PolicyResearchOnly || tool.InstallPolicy == analyzer.PolicyReferenceOnly {
			return
		}
		id := string(tool.ID)
		if seen[id] {
			return
		}
		seen[id] = true
		out = append(out, PaidPackToolProfile{
			ID:               id,
			Name:             tool.DisplayName,
			SourceURL:        tool.SourceURL,
			Category:         tool.Category,
			Reason:           string(rec.Reason),
			SignalIDs:        signalStrings(rec.SignalIDs),
			FailureModes:     failureModeStrings(rec, tool),
			Confidence:       string(rec.Confidence),
			RiskLevel:        string(nonEmptyRisk(rec.RiskLevel, tool.InstallRisk)),
			DataMovementRisk: string(nonEmptyRisk(rec.DataMovementRisk, tool.DataMovementRisk)),
			InstallPolicy:    string(tool.InstallPolicy),
			InstallSurface:   nonEmptyString(rec.InstallSurface, remediationInstallSurface(tool.ID, tool.Category)),
			RollbackGuidance: tool.RollbackGuidance,
			WaiverRequired:   toolWaiverRequired(rec, tool),
			VettingNotes:     nonEmptyString(rec.VettingNotes, tool.Notes),
		})
	}
	add(report.Recommendation.Primary)
	add(report.Recommendation.Secondary)
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func paidPackWaiver(tools []PaidPackToolProfile) PaidPackWaiver {
	var reasons []string
	for _, tool := range tools {
		if tool.WaiverRequired {
			reasons = append(reasons, tool.ID+": "+tool.InstallPolicy+" / "+tool.RiskLevel+" install risk / "+tool.DataMovementRisk+" data movement risk")
		}
	}
	sort.Strings(reasons)
	return PaidPackWaiver{
		Required: len(reasons) > 0,
		Reasons:  reasons,
		Language: "Before installing third-party tools or running Claude-executed setup commands, confirm the source URL, review permissions and data movement, pin versions when possible, and keep rollback instructions available.",
	}
}

func allowlistedIDs(category string, values []string) []string {
	seen := map[string]bool{}
	for _, value := range values {
		if safeIdentifier(value) && analyzer.ValidEcosystemID(category, value) {
			seen[value] = true
		}
	}
	out := make([]string, 0, len(seen))
	for value := range seen {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func isAttentionBand(band string) bool {
	switch band {
	case analyzer.WarningBandWatch, analyzer.WarningBandHigh, analyzer.WarningBandSevere:
		return true
	default:
		return false
	}
}

func signalStrings(signals []analyzer.Signal) []string {
	out := make([]string, 0, len(signals))
	for _, signal := range signals {
		out = append(out, string(signal))
	}
	sort.Strings(out)
	return out
}

func failureModeStrings(rec *analyzer.TokenSavingRecommendation, tool analyzer.TokenSavingTool) []string {
	if len(rec.FailureModes) > 0 {
		out := make([]string, 0, len(rec.FailureModes))
		for _, mode := range rec.FailureModes {
			out = append(out, string(mode))
		}
		sort.Strings(out)
		return out
	}
	out := remediationFailureModes(tool.RecommendationClass)
	sort.Strings(out)
	return out
}

func nonEmptyRisk(primary, fallback analyzer.RiskLevel) analyzer.RiskLevel {
	if primary != "" {
		return primary
	}
	return fallback
}

func nonEmptyString(primary, fallback string) string {
	if primary != "" {
		return primary
	}
	return fallback
}

func toolWaiverRequired(rec *analyzer.TokenSavingRecommendation, tool analyzer.TokenSavingTool) bool {
	return tool.InstallPolicy == analyzer.PolicyRecommendWithWaiver ||
		rec.InstallPolicy == analyzer.PolicyRecommendWithWaiver ||
		tool.InstallRisk == analyzer.RiskHigh ||
		tool.DataMovementRisk == analyzer.RiskHigh ||
		rec.RiskLevel == analyzer.RiskHigh ||
		rec.DataMovementRisk == analyzer.RiskHigh
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
