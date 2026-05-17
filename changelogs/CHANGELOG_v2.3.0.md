# Changelog v2.3.0 - Collaboration Suite

**Release Date**: May 7, 2026  
**Version**: 2.3.0  
**Status**: Production Ready

## 🎉 Overview

Botainer v2.3.0 introduces powerful collaboration features including multi-user support with role-based access control, a template library for sharing container configurations, and comprehensive audit logging. This release brings team collaboration capabilities to the Mini App.

---

## ✨ New Features

### 👥 Multi-User Support

**Backend** (`api/users.go`):
- User management system with role-based access control
- Three role levels:
  - **Admin**: Full access to all features including user management
  - **Operator**: Manage containers (no delete, no user management)
  - **Viewer**: Read-only access (logs, stats, metrics)
- Comprehensive audit logging system
- Automatic user creation on first access
- Last seen tracking
- Persistent storage in `/data/users.json`

**API Endpoints**:
- `GET /api/users` - List all users
- `PUT /api/users/{id}/role` - Update user role
- `GET /api/audit?limit=100` - Get audit log

**Frontend** (`UserManager.tsx`):
- 👥 User Management button in dashboard header
- Two-tab interface:
  - **Users Tab**: View and manage user roles
  - **Audit Log Tab**: View action history
- Role badges with color coding:
  - 🔴 Admin (red)
  - 🔵 Operator (blue)
  - ⚪ Viewer (gray)
- User information display:
  - Username and ID
  - Created date
  - Last seen timestamp
- Role dropdown for easy permission changes
- Audit log with:
  - User identification
  - Action performed
  - Resource affected
  - Success/failure indicator
  - Timestamp
  - Optional details

**Audit Log Tracking**:
- All container actions logged
- User role changes tracked
- Success/failure status
- Detailed action context
- Last 1,000 entries retained

---

### 📦 Template Library

**Backend** (`api/templates.go`):
- Save container configurations as reusable templates
- Template properties:
  - Name and description
  - Docker image
  - Port mappings
  - Volume mounts
  - Environment variables
  - Network configuration
  - Restart policy
  - Tags for categorization
- Public/private template visibility
- Usage tracking (deployment count)
- Creator attribution
- Persistent storage in `/data/templates.json`

**API Endpoints**:
- `GET /api/templates` - List templates
- `POST /api/templates` - Create template
- `GET /api/templates/{id}` - Get template details
- `DELETE /api/templates/{id}` - Delete template
- `POST /api/templates/{id}/deploy` - Deploy container from template

**Frontend** (`TemplateLibrary.tsx`):
- 📦 Template Library button in dashboard header
- Two-tab interface:
  - **Browse Templates**: View and deploy existing templates
  - **Create Template**: Save new container configurations
- Template cards showing:
  - Name and description
  - Docker image
  - Port mappings
  - Usage count
  - Tags
  - Public/private indicator
- One-click deployment:
  - Prompts for container name
  - Automatically creates and starts container
  - Increments usage counter
- Template creation form:
  - All container configuration options
  - Tag support for categorization
  - Public/private toggle
  - Input validation
- Template management:
  - Delete own templates
  - View usage statistics

**Use Cases**:
- Share common configurations across team
- Quick deployment of standard stacks
- Template marketplace for popular services
- Standardize container configurations
- Reduce deployment errors

---

## 🔧 Technical Improvements

### Architecture
- Added `UserStore` for user management
- Added `TemplateStore` for template management
- Extended `Server` struct with new stores
- Integrated audit logging throughout API

### Data Persistence
- `/data/users.json` - User data and audit log
- `/data/templates.json` - Template library
- Automatic file creation on first use
- JSON format for easy inspection

### Security
- Role-based permission checking
- User authentication via Telegram
- Audit trail for accountability
- Template ownership validation

---

## 📊 Statistics

### Code Added
- **Backend**: ~300 lines (Go)
  - `api/users.go`: 186 lines
  - `api/templates.go`: 116 lines
  - Handler additions: ~176 lines
- **Frontend**: ~596 lines (React/TypeScript)
  - `UserManager.tsx`: 214 lines
  - `TemplateLibrary.tsx`: 382 lines
- **Total**: ~896 lines of new code

### API Endpoints
- 3 user management endpoints
- 5 template library endpoints
- **Total new endpoints**: 8
- **Cumulative endpoints**: 21

### Components
- 2 new React components
- **Total components**: 8

---

## 🎯 Progress Summary

### Phase Completion
- **Phase 2.1** (Advanced Monitoring): ✅ 100% (3/3)
- **Phase 2.2** (Advanced Management): ✅ 100% (4/4)
- **Phase 2.3** (Collaboration): 🟡 40% (2/5)

### Overall Progress
- **Tasks Completed**: 16/20 (80%)
- **Features Implemented**: 16
- **Lines of Code**: ~3,400
- **API Endpoints**: 21
- **React Components**: 8

---

## 🚀 Deployment

### Update Steps

1. **Pull latest code**:
   ```bash
   cd /home/ubuntu/botainer
   git pull
   ```

2. **Build frontend**:
   ```bash
   cd webapp
   npm run build
   ```

3. **Rebuild and restart bot**:
   ```bash
   cd /home/ubuntu/botainer
   docker compose -f /home/ubuntu/chips_all/compose.yaml up -d --build botainer
   ```

4. **Verify deployment**:
   ```bash
   docker logs --tail 10 botainer
   ```

### Data Directories
Ensure `/data` volume is mounted for persistence:
- `/data/users.json` - User and audit data
- `/data/templates.json` - Template library
- `/data/metrics.json` - Historical metrics
- `/data/alerts.json` - Alert configurations

---

## 📖 Usage Guide

### Multi-User Management

1. **Access User Management**:
   - Open Mini App
   - Tap 👥 button in header

2. **Manage User Roles**:
   - View all users in Users tab
   - Select role from dropdown
   - Changes apply immediately

3. **View Audit Log**:
   - Switch to Audit Log tab
   - See all actions with timestamps
   - Filter by success/failure

### Template Library

1. **Browse Templates**:
   - Open Mini App
   - Tap 📦 button in header
   - View available templates

2. **Deploy from Template**:
   - Find desired template
   - Tap 🚀 Deploy button
   - Enter container name
   - Container created and started automatically

3. **Create Template**:
   - Switch to Create Template tab
   - Fill in configuration:
     - Name and description
     - Docker image
     - Ports, volumes, environment
     - Network and restart policy
     - Tags for categorization
   - Toggle public/private
   - Tap ✨ Create Template

4. **Manage Templates**:
   - View usage statistics
   - Delete own templates
   - Share public templates with team

---

## 🔮 What's Next

### Phase 2.3 Remaining (1 feature)
- **Advanced Updates**: Rollback, health checks, maintenance windows

### Phase 2.4 Planned
- Integration tests
- Performance optimizations
- Enhanced security features
- Advanced networking features

---

## 🐛 Known Issues

None reported in this release.

---

## 💡 Tips & Best Practices

### User Management
- Assign Admin role sparingly
- Use Operator role for daily operations
- Viewer role for monitoring-only users
- Review audit log regularly

### Template Library
- Use descriptive names and descriptions
- Add relevant tags for easy discovery
- Mark stable templates as public
- Test templates before sharing
- Document environment variables

---

## 🙏 Acknowledgments

Thanks to all users providing feedback and feature requests. Your input drives continuous improvement!

---

## 📞 Support

- **GitHub Issues**: https://github.com/YonierGomez/botainer/issues
- **Telegram Channel**: https://t.me/botainer_news
- **Documentation**: https://github.com/YonierGomez/botainer

---

## 📝 Version History

- **v2.4.1** (May 17, 2026) - Code quality, panic fixes, resolveComposeFile helper
- **v2.4.0** (May 17, 2026) - Logs fix, update reliability, new notification format
- **v2.3.1** (May 7, 2026) - Compact UI Redesign
- **v2.3.0** (May 7, 2026) - Collaboration Suite
- **v2.2.1** (May 6, 2026) - Complete Management Suite
- **v2.2.0** (May 6, 2026) - Advanced Management Release
- **v2.0.0** (May 6, 2026) - Mini App Initial Release

---

**Full Changelog**: https://github.com/YonierGomez/botainer/compare/v2.2.1...v2.3.0
