# Get disk template by permalink
data "katapult_disk_template" "ubuntu-20-04" {
  permalink = "templates/ubuntu-20-04"
}

# Get disk template by ID
data "katapult_disk_template" "ubuntu-18-04" {
  id = "dtpl_dHTYevsvwvsL0Hea"
}
