# 
# revad spec file
#

Name: revad
Summary: REVA for CERNBox
Version: 0.0.3
Release: 1%{?dist}
License: AGPLv3
BuildRoot: %{_tmppath}/%{name}-buildroot
Group: CERN-IT/ST
ExclusiveArch: x86_64
Source: %{name}-%{version}.tar.gz

%description
This RPM provides REVA for CERNBox, built from github.com/cernbox/reva

# Don't do any post-install weirdness, especially compiling .py files
%define __os_install_post %{nil}

%prep
%setup -n %{name}-%{version}

%install

# installation
rm -rf %buildroot/
mkdir -p %buildroot/usr/bin
mkdir -p %buildroot/etc/revad
# mkdir -p %buildroot/etc/logrotate.d
mkdir -p %buildroot%{_libdir}/systemd/system
mkdir -p %buildroot/var/log/revad
mkdir -p %buildroot/var/run/revad
install -m 755 revad	     %buildroot/usr/bin/revad
# install -m 644 revad.logrotate  %buildroot/etc/logrotate.d/revad

%clean
rm -rf %buildroot/

%preun

%post

%files
%defattr(-,root,root,-)
/etc/revad
# /etc/logrotate.d/revad
/var/log/revad
/var/run/revad
/usr/bin/*


%changelog
* Thu Jul 14 2022 Gianmaria Del Monte <gianmaria.del.monte@cern.ch> 0.0.2
- v0.0.2
* Thu Jul 07 2022 Hugo Gonzalez Labrador <hugo.gonzalez.labrador@cern.ch> 0.0.1
- v0.0.1

