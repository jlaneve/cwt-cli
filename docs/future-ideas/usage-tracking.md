# CWT Usage Tracking Integration

## Overview

This document outlines how to integrate Claude Code usage statistics directly into CWT, eliminating the need for separate tools like `ccusage`. By providing daily usage breakdowns alongside session management, users can track their Claude Code costs and usage patterns over time.

## Motivation

Currently, users need separate tools to track Claude Code usage and costs. Since CWT already manages Claude sessions and has access to the same JSONL files that ccusage reads, integrating usage tracking provides several benefits:

1. **Unified Experience**: Single tool for session management and usage tracking
2. **Daily Cost Visibility**: See how much you're spending each day across all sessions
3. **Usage Trends**: Track patterns and identify high-usage days
4. **Better Planning**: Understand daily burn rate for budget planning
5. **Historical Context**: Compare today's usage with previous days

## Technical Approach

### 1. Data Source: JSONL Files

Claude Code writes usage data to JSONL files in `~/.claude/projects/`. Each line contains:
- Token counts (input, output, cache read, cache creation)
- Model used (opus, sonnet, etc.)
- Timestamps
- Message IDs for deduplication
- Session and project identifiers

### 2. Architecture

```
┌─────────────────┐
│   CWT Session   │
│   Management    │
└────────┬────────┘
         │
┌────────▼────────┐    ┌─────────────────┐
│  Usage Tracker  │───▶│  JSONL Parser   │
│    Module       │    │                 │
└────────┬────────┘    └─────────────────┘
         │
┌────────▼────────┐    ┌─────────────────┐
│  Cost Calculator│───▶│ Pricing Service │
│                 │    │  (LiteLLM/Cache)│
└────────┬────────┘    └─────────────────┘
         │
┌────────▼────────┐
│  TUI Dashboard  │
│  Usage Panel    │
└─────────────────┘
```

### 3. Implementation Plan

#### Phase 1: Core Usage Tracking

**New Package: `internal/usage/`**

```go
// internal/usage/tracker.go
package usage

type Tracker struct {
    projectsDir string
    dailyUsage  map[string]*DailyUsage  // key: "2024-01-15"
    pricing     *PricingService
}

type DailyUsage struct {
    Date             string
    TotalInputTokens int64
    TotalOutputTokens int64
    TotalCacheRead   int64
    TotalCacheCreate int64
    TotalCost        float64
    ModelBreakdown   map[string]*ModelUsage
    HourlyUsage      [24]float64  // Cost per hour
    SessionCount     int           // Number of unique sessions
}

type ModelUsage struct {
    Model            string
    InputTokens      int64
    OutputTokens     int64
    CacheReadTokens  int64
    CacheCreateTokens int64
    Cost             float64
    RequestCount     int
}

// Get usage for a specific date
func (t *Tracker) GetDailyUsage(date time.Time) (*DailyUsage, error) {
    // 1. Scan all JSONL files for entries on this date
    // 2. Aggregate token counts by model
    // 3. Calculate total costs
    // 4. Return daily summary
}

// Get usage for date range
func (t *Tracker) GetUsageRange(from, to time.Time) ([]*DailyUsage, error) {
    // Return usage for each day in range
}
```

**JSONL Parser:**

```go
// internal/usage/parser.go
type JSONLEntry struct {
    Type      string    `json:"type"`
    Timestamp time.Time `json:"timestamp"`
    Message   struct {
        ID      string `json:"id"`
        Model   string `json:"model"`
        Usage   struct {
            InputTokens       int `json:"input_tokens"`
            OutputTokens      int `json:"output_tokens"`
            CacheReadTokens   int `json:"cache_read_tokens"`
            CacheCreateTokens int `json:"cache_creation_tokens"`
        } `json:"usage"`
    } `json:"message"`
}

func ParseJSONLFile(path string, afterTimestamp time.Time) ([]JSONLEntry, error) {
    // Read file line by line
    // Parse each JSON line
    // Filter entries after timestamp
    // Return parsed entries
}
```

**Pricing Service:**

```go
// internal/usage/pricing.go
type PricingService struct {
    prices map[string]ModelPricing
    cache  *PricingCache
}

type ModelPricing struct {
    InputCost        float64 // per 1M tokens
    OutputCost       float64 // per 1M tokens
    CacheReadCost    float64 // per 1M tokens
    CacheCreateCost  float64 // per 1M tokens
}

func (p *PricingService) CalculateCost(model string, usage TokenUsage) float64 {
    // Look up model pricing
    // Calculate cost based on token counts
    // Return total cost
}
```

#### Phase 2: TUI Integration

**Usage Dashboard View:**

```go
// internal/tui/usage_view.go
type UsageView struct {
    tracker     *usage.Tracker
    currentDate time.Time
    dateRange   int // days to show
}

func (u *UsageView) View() string {
    // Display daily usage stats:
    // - Bar chart of last 7 days
    // - Today's total cost
    // - Model breakdown
    // - Hourly usage pattern
}
```

**Enhanced Dashboard Layout:**

```
┌─ Sessions ─────────────────────────┐┌─ Daily Usage ──────────────────┐
│ 🟢 auth-system                     ││ Today: $3.42                   │
│    └─ Implementing JWT auth...     ││ Yesterday: $2.18               │
│ 🟡 payment-flow                    ││ This Week: $15.73              │
│    └─ Waiting for input...         ││                                │
│ ✅ bug-fix                         ││ Last 7 Days:                   │
│    └─ Fixed login redirect         ││ ██████ $3.42 Today             │
│                                    ││ ████   $2.18 Yesterday         │
└────────────────────────────────────┘│ ███    $1.95 Jan 13            │
                                      │ █████  $2.87 Jan 12            │
[u] Show usage details                │ ██     $1.23 Jan 11            │
[s] Back to sessions                  │ ████   $2.14 Jan 10            │
                                      │ ███    $1.94 Jan 09            │
                                      └────────────────────────────────┘
```

**Detailed Usage View (press 'u'):**

```
┌─ Usage Details - January 15, 2024 ─────────────────────────────────────┐
│ Total Cost: $3.42                    Sessions: 5                       │
│                                                                        │
│ By Model:                            By Hour:                          │
│ ├─ claude-3-opus:    $2.87          00:00 ▁▁▁▁▁▁▁▁▁▁                │
│ └─ claude-3-sonnet:  $0.55          06:00 ▃▄▅▆▇█▇▅▃▁                │
│                                      12:00 ▁▂▃▄▅▆▇▆▅▄                │
│ Token Usage:                         18:00 ▃▂▁▁▁▁▁▁▁▁                │
│ ├─ Input:     234,567                                                 │
│ ├─ Output:     89,123               Peak Usage: 2-3 PM ($0.67)        │
│ ├─ Cache Read: 45,678                                                 │
│ └─ Cache Create: 12,345             Monthly Projection: $102.60       │
└────────────────────────────────────────────────────────────────────────┘
[←/→] Previous/Next Day    [w] Week View    [m] Month View    [ESC] Back
```

#### Phase 3: Advanced Features

**1. Daily Budget Tracking:**
```go
type BudgetConfig struct {
    DailyLimit   float64
    WeeklyLimit  float64
    MonthlyLimit float64
    AlertAt      float64  // Percentage (e.g., 0.8 for 80%)
}

// Check if daily budget is exceeded
func (t *Tracker) CheckDailyBudget() *BudgetAlert {
    today := t.GetDailyUsage(time.Now())
    config := t.config.Budget
    
    if today.TotalCost >= config.DailyLimit*config.AlertAt {
        return &BudgetAlert{
            Level: "warning",
            Message: fmt.Sprintf("Daily usage at $%.2f (%.0f%% of $%.2f limit)", 
                today.TotalCost, 
                (today.TotalCost/config.DailyLimit)*100,
                config.DailyLimit),
        }
    }
    return nil
}
```

**2. Usage Trends & Analytics:**
```go
// Analyze usage patterns
type UsageTrends struct {
    AverageDailyCost    float64
    WeekdayAverage      map[string]float64  // Mon: $2.50, Tue: $3.20, etc
    PeakUsageHours      []int               // [14, 15, 16] = 2-5 PM
    ModelPreference     map[string]float64  // opus: 70%, sonnet: 30%
    GrowthRate          float64             // % change week-over-week
}

func (t *Tracker) AnalyzeTrends(days int) *UsageTrends {
    // Look at last N days
    // Calculate patterns
    // Return insights
}
```

**3. Real-time Daily Updates:**
```go
// Monitor today's usage in real-time
func (t *Tracker) WatchTodayUsage() <-chan DailyUpdate {
    updates := make(chan DailyUpdate)
    
    go func() {
        ticker := time.NewTicker(30 * time.Second)
        defer ticker.Stop()
        
        for range ticker.C {
            today := t.GetDailyUsage(time.Now())
            updates <- DailyUpdate{
                Cost:         today.TotalCost,
                TokensUsed:   today.TotalInputTokens + today.TotalOutputTokens,
                LastUpdated:  time.Now(),
            }
        }
    }()
    
    return updates
}
```

**4. Export & Reporting:**
```bash
# Show today's usage
cwt usage
# Output:
# Daily Usage Summary
# ─────────────────────────────────────────────────────────
# Today (Jan 15):     $3.42    ████████████████ 
# Yesterday:          $2.18    ██████████
# Jan 13:             $1.95    █████████
# Jan 12:             $2.87    █████████████
# Jan 11:             $1.23    ██████
# ─────────────────────────────────────────────────────────
# Week Total:         $15.73   Daily Average: $2.25

# Show detailed daily breakdown
cwt usage --day=today
# Output:
# Usage for January 15, 2024
# ─────────────────────────────────────────────────────────
# Total Cost: $3.42
# 
# By Model:
#   claude-3-opus:     $2.87 (83.9%)
#   claude-3-sonnet:   $0.55 (16.1%)
#
# Token Usage:
#   Input:        234,567
#   Output:        89,123
#   Cache Read:    45,678
#   Cache Create:  12,345
#
# Hourly Distribution:
#   09:00-10:00:  $0.23
#   10:00-11:00:  $0.45
#   11:00-12:00:  $0.38
#   ...

# Export usage data
cwt usage export --format=csv --from=2024-01-01 --to=2024-01-31
```

### 4. User Experience

#### In TUI Dashboard:
- **Daily usage panel**: Shows last 7 days as bar chart
- **Today's total**: Prominently displayed with live updates
- **Trends**: Visual indicators for usage patterns
- **Budget alerts**: Warning when approaching daily limits

#### Dedicated Usage View:
- Press `u` from main dashboard to see detailed usage
- Navigate between days with arrow keys
- Switch between daily/weekly/monthly views
- See hourly usage patterns and model breakdown

### 5. Configuration

```yaml
# .cwt/config.yaml
usage:
  track_usage: true
  projects_dir: "~/.claude/projects"  # Override default location
  
  budgets:
    daily_limit: 10.00                # $10 per day limit
    weekly_limit: 50.00               # $50 per week limit
    monthly_limit: 200.00             # $200 per month limit
    alert_threshold: 0.8              # Alert at 80% of budget
    
  reporting:
    show_in_tui: true                 # Show usage panel in dashboard
    default_view: "week"              # week, month, or day
    update_interval: 30               # Seconds between updates
    
  export:
    format: "json"                    # json, csv, or table
    include_hourly: true              # Include hourly breakdown
```

### 6. Benefits Over Separate Tool

1. **Integrated Experience**: Usage data alongside session management
2. **Daily Focus**: Better cost awareness with daily breakdowns
3. **Trend Analysis**: See patterns in your Claude usage over time
4. **Budget Management**: Set and track daily/weekly/monthly limits
5. **Quick Access**: Press 'u' in dashboard for instant usage view

### 7. Migration Path

For users currently using ccusage:
1. CWT can import historical data from existing JSONL files
2. Provide similar command-line interface for familiarity
3. Enhanced functionality through session integration
4. No loss of existing data or reporting capabilities

## Implementation Priority

1. **High Priority**:
   - Basic usage tracking and cost calculation
   - TUI integration showing costs per session
   - Real-time monitoring during active sessions

2. **Medium Priority**:
   - Budget management and alerts
   - Usage analytics and reporting
   - Export functionality

3. **Low Priority**:
   - Historical data import
   - Advanced analytics
   - Cost optimization suggestions

## Conclusion

Integrating daily usage tracking into CWT provides a natural extension to session management. By focusing on daily breakdowns rather than per-session tracking, users get a clearer picture of their spending patterns and can better manage their Claude Code usage over time. The integration eliminates the need for separate tools while providing richer insights through the TUI dashboard.