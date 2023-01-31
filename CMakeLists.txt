CMAKE_MINIMUM_REQUIRED (VERSION 3.13)
PROJECT (cbsummary)

# This project is intended to be built as part of a Couchbase Server build,
# but for nominal support building stand-alone, the following variables
# need to be specified:
#
# CMAKE_MODULE_PATH - should contain a directory containing
#      FindCouchbaseGo.cmake, eg. tlm/cmake/Modules
#
# CMAKE_INSTALL_PREFIX - should point to a directory where the bin/ output
#      directory will be placed
#
# GODEPSDIR - should point to a Go workspace directory containing all
#      transitive Go dependencies
#

IF (BUILD_ENTERPRISE)
  INCLUDE (FindCouchbaseGo)

  IF (WIN32)
    INCLUDE (PlatformIntrospection)
    _DETERMINE_ARCH (_arch)
    IF (NOT _arch STREQUAL x86)
      SET (_ldflags "-linkmode internal")
    ENDIF ()
  ENDIF ()

  # Get the commit hash
  IF (EXISTS ${CMAKE_CURRENT_SOURCE_DIR}/.git)
    EXECUTE_PROCESS(COMMAND git rev-parse --short HEAD
                    WORKING_DIRECTORY ${CMAKE_CURRENT_SOURCE_DIR}
                    OUTPUT_VARIABLE _version_hash
                    OUTPUT_STRIP_TRAILING_WHITESPACE)
    IF (_version_hash STREQUAL "" OR _version_hash STREQUAL "HEAD")
      SET(_version_hash "unknown")
    ENDIF (_version_hash STREQUAL "" OR _version_hash STREQUAL "HEAD")
  ELSE (EXISTS ${CMAKE_CURRENT_SOURCE_DIR}/.git)
    SET(_version_hash "unknown")
  ENDIF (EXISTS ${CMAKE_CURRENT_SOURCE_DIR}/.git)

  SET (_ldflags " ${_ldflags} -X main.installType=couchbase -X main.versionHash=${_version_hash}")
  IF (APPLE)
    # On OS X 10.11 (El Capitan) upwards we can no longer use DYLD_LIBRARY_PATH to locate
    # runtime dependancies.
    SET (_ldflags " ${_ldflags} -extldflags '-Wl,-rpath,@executable_path/../lib'")
  ENDIF ()

  GoInstall (TARGET cbsummary PACKAGE github.com/couchbase/cbsummary
    GOPATH "${PROJECT_SOURCE_DIR}/../../../.." "${GODEPSDIR}"
    LDFLAGS "${_ldflags}"
    INSTALL_PATH bin OUTPUT cbsummary
    GOVERSION 1.19)

#  ADD_SUBDIRECTORY(docs)

ENDIF(BUILD_ENTERPRISE)
