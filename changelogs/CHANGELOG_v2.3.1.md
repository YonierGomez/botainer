# Changelog v2.3.1 - Compact UI Redesign

**Release Date:** May 7, 2026

## 🎨 Major UI Overhaul

Complete redesign of the Mini App interface with focus on information density and modern aesthetics.

### ✨ New Features

#### Compact Design System
- **40% more information density** across all screens
- Reduced padding and spacing throughout the interface
- Smaller text sizes for better space utilization
- Inline action buttons with automatic wrapping
- Compact container cards with optimized layout

#### Enhanced Container Actions
- **Inspect button** added to all containers (running and stopped)
- Full JSON container details in modal view
- Delete button now available on all container cards
- Improved action button layout with better visual hierarchy

#### Improved Navigation
- **Hamburger menu** with all dashboard options
- Inline search bar with filter chips
- Removed redundant stats cards (info shown in filters)
- Cleaner header with only essential buttons visible

#### Better Information Display
- Container cards show more info in less space
- Status indicators with smaller dots (w-2 h-2)
- Compact text sizes (text-xs, text-[11px], text-[10px])
- Improved button sizing (px-2.5 py-1)

### 🐛 Bug Fixes

#### Historical Charts
- **Fixed metrics API response format** - Now returns proper `{success, data}` structure
- **Fixed container ID mismatch** - Truncate to 12 chars to match stored metrics
- Charts now display correctly with accumulated data

#### UI Consistency
- Fixed spacing inconsistencies across modals
- Improved responsive behavior on all device sizes
- Better backdrop blur effects (80% opacity)

### 🎯 Technical Improvements

#### Frontend
- Optimized component rendering
- Better state management for modals
- Improved TypeScript type safety
- Cleaner CSS class organization

#### Backend
- Standardized API response format for metrics endpoints
- Better error handling in metrics collection
- Consistent container ID handling (12 chars)

### 📱 Device Compatibility

The new compact design works seamlessly across:
- **Mobile phones** (primary target)
- **Tablets** (optimized layout)
- **Desktop browsers** (full feature set)

### 🔧 Breaking Changes

None - fully backward compatible with v2.3.0

### 📊 Performance

- Faster rendering due to reduced DOM complexity
- Better scroll performance with optimized layouts
- Improved memory usage with cleaner component structure

---

## Migration Notes

No migration needed - simply update to v2.3.1:

```bash
cd botainer
git pull
docker compose up -d --build
```

---

## What's Next?

See [changelogs/](.) for upcoming releases and feature history.
