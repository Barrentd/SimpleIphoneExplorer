$s = New-Object -ComObject Shell.Application
Write-Output "=== NameSpace(17) - My Computer ==="
$c = $s.NameSpace(17)
foreach($i in $c.Items()) {
    Write-Output ("  " + $i.Name + " | IsFolder: " + $i.IsFolder + " | Path: " + $i.Path)
}

Write-Output ""
Write-Output "=== Checking for Portable Devices ==="
# Try NameSpace with Portable Devices CLSID
try {
    $pd = $s.NameSpace("::{35786D3C-B075-49b9-88DD-029876E11C01}")
    if ($pd) {
        foreach($i in $pd.Items()) {
            Write-Output ("  " + $i.Name + " | IsFolder: " + $i.IsFolder)
        }
    } else {
        Write-Output "  No portable devices namespace"
    }
} catch {
    Write-Output ("  Error: " + $_.Exception.Message)
}
