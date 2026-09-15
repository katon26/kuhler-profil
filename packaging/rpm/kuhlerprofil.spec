Name:           kuhlerprofil
Version:        0.1.0
Release:        1%{?dist}
Summary:        ASUS Linux thermal profile governor, fan curves, and battery health guardian

License:        MIT
URL:            https://github.com/katon26/kuhler-profil
Source0:        %{name}-%{version}.tar.gz

BuildRequires:  make
BuildRequires:  golang >= 1.22
Requires:       dbus
Requires:       systemd

%description
KühlerProfil is a native Linux background daemon, CLI, and GNOME Shell
extension for ASUS VivoBook, ZenBook, TUF, and ROG laptops.
It provides hardware ACPI thermal profile control (Silent, Standard, Boost),
multi-point RPM fan curve programming, Zero-RPM cooldown recovery, and
battery health charging threshold limits (60%, 80%, 100%).

%prep
%autosetup

%build
make build

%install
rm -rf $RPM_BUILD_ROOT
install -d $RPM_BUILD_ROOT%{_bindir}
install -m 755 bin/kuhlerprofild $RPM_BUILD_ROOT%{_bindir}/kuhlerprofild
install -m 755 bin/kuhlerprofil $RPM_BUILD_ROOT%{_bindir}/kuhlerprofil
ln -sf kuhlerprofil $RPM_BUILD_ROOT%{_bindir}/kp
ln -sf kuhlerprofil $RPM_BUILD_ROOT%{_bindir}/kuhler

install -d $RPM_BUILD_ROOT%{_unitdir}
install -m 644 systemd/kuhlerprofil.service $RPM_BUILD_ROOT%{_unitdir}/kuhlerprofil.service

install -d $RPM_BUILD_ROOT%{_sysconfdir}/dbus-1/system.d
install -m 644 systemd/org.freedesktop.kuhlerprofil.conf $RPM_BUILD_ROOT%{_sysconfdir}/dbus-1/system.d/org.freedesktop.kuhlerprofil.conf

install -d $RPM_BUILD_ROOT%{_sysconfdir}/kuhlerprofil

%post
%systemd_post kuhlerprofil.service
if [ $1 -eq 1 ]; then
    # Initial installation: reload dbus
    systemctl reload dbus.service >/dev/null 2>&1 || true
fi

%preun
%systemd_preun kuhlerprofil.service

%postun
%systemd_postun_with_restart kuhlerprofil.service

%files
%license LICENSE
%doc README.md
%{_bindir}/kuhlerprofild
%{_bindir}/kuhlerprofil
%{_bindir}/kp
%{_bindir}/kuhler
%{_unitdir}/kuhlerprofil.service
%config(noreplace) %{_sysconfdir}/dbus-1/system.d/org.freedesktop.kuhlerprofil.conf
%dir %{_sysconfdir}/kuhlerprofil

%changelog
* Sun Sep 15 2026 Katon <katonadams.f@gmail.com> - 0.1.0-1
- Initial 0.1.0 release with thermal profiles, fan curves, zero-rpm cooldown, and battery limiter.
