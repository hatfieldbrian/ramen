# VolumeReplicationGroup (VRG) CR Status Conditions

This page outlines the various conditions reported by the VolumeReplicationGroup (VRG) CR that is deployed on the DR cluster. For the hub cluster related CR conditions, see [DRPC CR Status Conditions](drpc-status-conditions.md) and [DRPolicy CR Status Conditions](drpolicy-status-conditions.md).

```mermaid
stateDiagram-v2

direction LR
VolumeReplicationGroup : VolumeReplicationGroup (VRG) CR Status Overview

state VolumeReplicationGroup {
    DataRelatedConditions : VRG Data related conditions
    ClusterDataRelatedConditions : VRG ClusterData related conditions

    VRGStatus : Status

    ProtectedPVCs : Status of the set of PVCs \nprotected by the VRG
    VRGLevelConditions : A set of VRG-level conditions
    PVCLevelConditions : A set of PVC-level conditions \n (one per PVC)


    VRGStatus --> VRGLevelConditions : contains
    VRGStatus --> ProtectedPVCs: contains
    ProtectedPVCs --> PVCLevelConditions : each PVC contains

    VRGLevelConditions --> VRGClusterDataRelatedConditions : contains
    VRGLevelConditions --> VRGDataRelatedConditions : contains
    VRGLevelConditions --> PVCLevelConditions : mostly depend on
    PVCLevelConditions --> VRGClusterDataRelatedConditions : contains
    PVCLevelConditions --> VRGDataRelatedConditions : contains
}


state ClusterDataRelatedConditions {
    direction LR
    ClusterDataProtectedCondition : ClusterDataProtected condition
    ClusterDataReadyCondition : ClusterDataReady condition
    CDPFalse : False
    CDPTrue : True
    CDPUnknown : Unknown
    CDPInitializing : Initializing
    CDPUploadError : Upload error
    CDPUploading : Uploading
    CDPUploaded : Uploaded
    CDRFalse : False
    CDRTrue : True
    CDRUnknown : Unknown

    state ClusterDataProtectedCondition {
        direction LR

        state CDPUnknown {
            [*] --> CDPInitializing
        }

        state CDPFalse {
            CDPInitializing --> CDPUploadError
            CDPInitializing --> CDPUploaded : AppState in \n PVCs only
            CDPInitializing --> CDPUploading : AppState in \n API server
            CDPUploaded --> CDPUploadError
            CDPUploadError --> CDPUploading
            CDPUploadError --> CDPUploaded
            CDPUploading --> CDPUploadError
        }

        state CDPTrue {
            CDPUploading --> CDPUploaded
        }

    }

    state ClusterDataReadyCondition {
        direction LR

        state CDRUnknown {
            [*] --> CDRInitializing
        }

        state CDRFalse {
            CDRInitializing --> CDRRestored

            CDRInitializing : Initializing
        }

        state CDRTrue {
            CDRRestored : Restored
        }
    }
}

state DataRelatedConditions {
    direction LR
    DataReadyCondition : DataReady condition
    DataProtectedCondition : DataProtected condition
    DPFalse : False
    DPTrue : True
    DPUnknown : Unknown
    DPInitializing : Initializing
    DPReplicating : Replicating
    DPProtected: DataProtected
    DPError: Error
    DPReady : Ready
    DRFalse : False
    DRTrue : True
    DRUnknown : Unknown
    DRInitializing : Initializing
    DRReplicating : Replicating
    DRReplicated : Replicated
    DRReady : Ready
    DRProgressing : Progressing
    DRError: Error
    DRUnknownError: Unknown error

    state DataProtectedCondition {
        direction LR

        state DPUnknown {
            [*] --> DPInitializing
        }


        state DPTrue {
            state if_sync <<choice>>
            state if_primary <<choice>>

            DPInitializing --> if_sync
            if_sync --> DPReady : sync
            if_sync --> if_primary : async

            if_primary --> DPProtected
            DPReplicating --> DPProtected
        }

        state DPFalse {
            DPInitializing --> DPError
            DPReplicating --> DPError
        }

    }

    state DataReadyCondition {
        direction LR

        state DRUnknown {
            [*] --> DRInitializing
        }

        state DRFalse {
            DRInitializing --> DRError
            DRInitializing --> DRReplicating
            DRReplicating --> DRReplicated
            DRReplicating --> DRProgressing
            DRReplicating --> DRError
            DRReplicating --> DRUnknownError
        }
        state DRTrue {
            DRReplicating --> DRReady
        }

    }
}

note right of DataRelatedConditions
    DataProtected condition:
    - becomes true when the set of PVCs of the
    VRG are in a protected state (DataProtected),
    an indication that the DR relationship with the
    DR peer cluster is healthy.
end note

note right of DataRelatedConditions
    DataReady condition:
    - becomes true when
end note

note right of ClusterDataRelatedConditions
    ClusterDataProtected condition:
    - becomes true when VRG successfully
    uploads cluster data of PVs ("Uploaded").
    - "Upload error" results either when
    VRG has no S3 stores configured or when
    VRG is unable to upload to any of the S3 stores.
    - The "Uploading" state is defined but not used.
end note

note right of ClusterDataRelatedConditions
    ClusterDataReady condition:
    - becomes true when cluster data
    of PVs are "Restored" from the S3 store
    to the API server when VRG is created
    in "Primary" state
    - is a VRG level condition and is
    not reported for each PVC.
end note




```